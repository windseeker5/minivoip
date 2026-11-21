package notes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minivoip/store"
)

// openRouterRequest is what the fake OpenRouter got.
type openRouterRequest struct {
	Path, Key string
	Body      map[string]any
	Audio     string
}

func fakeOpenRouter(t *testing.T, answer func(r openRouterRequest) string) (*httptest.Server, *[]openRouterRequest) {
	t.Helper()
	var got []openRouterRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := openRouterRequest{Path: r.URL.Path, Key: r.Header.Get("Authorization")}
		if err := json.NewDecoder(r.Body).Decode(&req.Body); err != nil {
			t.Errorf("not JSON: %v", err)
		}
		if in, ok := req.Body["input_audio"].(map[string]any); ok {
			b, err := base64.StdEncoding.DecodeString(in["data"].(string))
			if err != nil {
				t.Errorf("audio: %v", err)
			}
			req.Audio = in["format"].(string) + ":" + string(b)
		}
		got = append(got, req)
		io.WriteString(w, answer(req))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestOpenRouterScribeVoicemail(t *testing.T) {
	srv, got := fakeOpenRouter(t, func(openRouterRequest) string {
		return `{"text":"x","duration":11.2,"words":[
			{"word":"Bonjour","start":0.5,"end":1,"speaker":0,"speaker_label":"speaker_0"},
			{"word":"Joe,","start":1.1,"end":1.5,"speaker":0,"speaker_label":"speaker_0"},
			{"word":"(rires)","type":"audio_event","start":2,"end":3,"speaker":1},
			{"word":"Salut","start":3,"end":4,"speaker":1},
			{"word":"Bye","start":5,"end":6,"speaker":0,"speaker_label":"speaker_0"}]}`
	})
	file := filepath.Join(t.TempDir(), "msg.m4a")
	os.WriteFile(file, []byte("sound"), 0o600)
	tr, err := (&OpenRouterScribe{Key: "secret", URL: srv.URL}).Transcribe(context.Background(), file, "audio/mp4", nil)
	if err != nil {
		t.Fatal(err)
	}
	r := (*got)[0]
	provider, _ := r.Body["provider"].(map[string]any)
	if r.Path != "/audio/transcriptions" || r.Key != "Bearer secret" || r.Body["model"] != openRouterScribeModel ||
		r.Body["diarize"] != true || r.Body["response_format"] != "verbose_json" || provider["data_collection"] != "deny" {
		t.Errorf("request: %+v", r)
	}
	if r.Audio != "m4a:sound" {
		t.Errorf("audio: %q", r.Audio)
	}
	want := []store.NoteLine{{Speaker: "speaker_0", Start: 0.5, Text: "Bonjour Joe,"}, {Speaker: "speaker_1", Start: 3, Text: "Salut"}, {Speaker: "speaker_0", Start: 5, Text: "Bye"}}
	if len(tr.Lines) != len(want) {
		t.Fatalf("lines: %+v", tr.Lines)
	}
	for i := range want {
		if tr.Lines[i] != want[i] {
			t.Errorf("line %d: %+v, want %+v", i, tr.Lines[i], want[i])
		}
	}
	if tr.Seconds != 12 {
		t.Errorf("seconds: %d", tr.Seconds)
	}
}

// A call: ffmpeg cuts out each channel, each is written down on its own, and the
// lines say owner and caller, in the order they were said.
func TestOpenRouterScribeCallChannels(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := filepath.Join(bin, "ffmpeg")
	os.WriteFile(ffmpeg, []byte(`#!/bin/sh
for a; do last="$a"; done
case "$*" in *c0=c1*) printf caller > "$last";; *) printf owner > "$last";; esac
`), 0o700)
	srv, got := fakeOpenRouter(t, func(r openRouterRequest) string {
		if strings.HasSuffix(r.Audio, "caller") {
			return `{"duration":20,"words":[{"word":"Bonjour,","start":0.5,"end":1},{"word":"Marie","start":1.1,"end":1.5},{"word":"Merci","start":11,"end":11.5}]}`
		}
		return `{"duration":20,"words":[{"word":"Oui","start":2,"end":2.5}]}`
	})
	file := filepath.Join(t.TempDir(), "call.webm")
	os.WriteFile(file, []byte("stereo"), 0o600)
	tr, err := (&OpenRouterScribe{Key: "k", URL: srv.URL, FFmpeg: ffmpeg}).Transcribe(context.Background(), file, "audio/webm", CallChannels)
	if err != nil {
		t.Fatal(err)
	}
	if len(*got) != 2 {
		t.Fatalf("requests: %d", len(*got))
	}
	for _, r := range *got {
		if _, ok := r.Body["diarize"]; ok {
			t.Error("one voice per channel needs no telling apart")
		}
	}
	want := []store.NoteLine{{Speaker: "caller", Start: 0.5, Text: "Bonjour, Marie"}, {Speaker: "owner", Start: 2, Text: "Oui"}, {Speaker: "caller", Start: 11, Text: "Merci"}}
	if len(tr.Lines) != len(want) {
		t.Fatalf("lines: %+v", tr.Lines)
	}
	for i := range want {
		if tr.Lines[i] != want[i] {
			t.Errorf("line %d: %+v, want %+v", i, tr.Lines[i], want[i])
		}
	}
	if tr.Seconds != 20 {
		t.Errorf("seconds: %d (the recording's length, not both channels)", tr.Seconds)
	}

	// Without ffmpeg, the whole call goes once and the service tells voices apart.
	*got = nil
	if _, err := (&OpenRouterScribe{Key: "k", URL: srv.URL, FFmpeg: filepath.Join(bin, "none")}).Transcribe(context.Background(), file, "audio/webm", CallChannels); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0].Body["diarize"] != true || (*got)[0].Audio != "webm:stereo" {
		t.Errorf("without ffmpeg: %+v", *got)
	}

	// A recording ffmpeg can't read won't read on another try.
	os.WriteFile(ffmpeg, []byte("#!/bin/sh\necho broken >&2\nexit 1\n"), 0o700)
	if _, err := (&OpenRouterScribe{Key: "k", URL: srv.URL, FFmpeg: ffmpeg}).Transcribe(context.Background(), file, "audio/webm", CallChannels); !errors.Is(err, ErrPermanent) {
		t.Errorf("broken recording: %v", err)
	}
}

func TestOpenRouterScribeTextOnly(t *testing.T) {
	srv, _ := fakeOpenRouter(t, func(openRouterRequest) string { return `{"text":" Bonjour Joe ","usage":{"seconds":3}}` })
	file := filepath.Join(t.TempDir(), "msg.webm")
	os.WriteFile(file, []byte("sound"), 0o600)
	tr, err := (&OpenRouterScribe{Key: "k", URL: srv.URL}).Transcribe(context.Background(), file, "audio/webm", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Lines) != 1 || tr.Lines[0].Text != "Bonjour Joe" || tr.Seconds != 3 {
		t.Errorf("transcript: %+v", tr)
	}
}

func TestOpenRouterChat(t *testing.T) {
	srv, got := fakeOpenRouter(t, func(openRouterRequest) string {
		return `{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":\" Marie veut la chambre bleue. \",\"next_actions\":[\"Rappeler Marie\"]}"}}]}`
	})
	note, err := (&OpenRouterChat{Key: "secret", URL: srv.URL}).Summarize(context.Background(), twoLines.Lines, store.French, []string{"Marie"})
	if err != nil {
		t.Fatal(err)
	}
	if note.Text != "Marie veut la chambre bleue." || len(note.NextActions) != 1 {
		t.Errorf("note: %+v", note)
	}
	r := (*got)[0]
	provider, _ := r.Body["provider"].(map[string]any)
	format, _ := r.Body["response_format"].(map[string]any)
	if r.Path != "/chat/completions" || r.Key != "Bearer secret" || r.Body["model"] != openRouterNoteModel ||
		provider["data_collection"] != "deny" || provider["require_parameters"] != true || format["type"] != "json_schema" {
		t.Errorf("request: %+v", r)
	}
	if _, ok := r.Body["tools"]; ok {
		t.Error("no tools")
	}
	messages, _ := r.Body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages: %+v", messages)
	}
	system := messages[0].(map[string]any)["content"].(string)
	user := messages[1].(map[string]any)["content"].(string)
	if system != notePrompt(store.French) || !strings.Contains(user, "<transcript>") || !strings.Contains(user, "Marie") {
		t.Errorf("messages: %q / %q", system, user)
	}
}

func TestOpenRouterChatAnswers(t *testing.T) {
	for answer, permanent := range map[string]bool{
		`{"choices":[{"finish_reason":"stop","message":{"refusal":"no"}}]}`:                  true,
		`{"choices":[{"finish_reason":"length","message":{"content":"{\"summ"}}]}`:           true,
		`{"choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`:          true,
		`{"choices":[{"finish_reason":"stop","message":{"content":"{\"summary\":\" \"}"}}]}`: false,
		`{"choices":[]}`:               false,
		`{"error":{"message":"busy"}}`: false,
	} {
		_, err := readOpenRouterAnswer([]byte(answer))
		if err == nil || errors.Is(err, ErrPermanent) != permanent {
			t.Errorf("%s: %v", answer, err)
		}
	}
}

func TestOpenRouterErrorsAndRedirects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "msg.webm")
	os.WriteFile(file, []byte("sound"), 0o600)
	for status, permanent := range map[int]bool{400: true, 401: false, 402: false, 429: false, 502: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(status)
			io.WriteString(w, strings.Repeat("no", 1000))
		}))
		_, scribeErr := (&OpenRouterScribe{Key: "k", URL: srv.URL}).Transcribe(context.Background(), file, "audio/webm", nil)
		_, chatErr := (&OpenRouterChat{Key: "k", URL: srv.URL}).Summarize(context.Background(), twoLines.Lines, store.French, nil)
		srv.Close()
		for _, err := range []error{scribeErr, chatErr} {
			if err == nil || errors.Is(err, ErrPermanent) != permanent || len(err.Error()) > 500 {
				t.Errorf("status %d: %v", status, err)
			}
		}
	}

	var reached bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if _, err := (&OpenRouterScribe{Key: "k", URL: srv.URL}).Transcribe(context.Background(), file, "audio/webm", nil); err == nil {
		t.Error("scribe: a redirect counted as an answer")
	}
	if _, err := (&OpenRouterChat{Key: "k", URL: srv.URL}).Summarize(context.Background(), twoLines.Lines, store.French, nil); err == nil {
		t.Error("chat: a redirect counted as an answer")
	}
	if reached {
		t.Error("the key went to another address")
	}
}

// Each service's own key wins; OpenRouter's covers what's missing, before the demo.
func TestOpenRouterFromEnv(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"uv", "claude"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o700)
	}
	t.Setenv("PATH", bin)
	t.Setenv("MINIVOIP_NOTES_LOCAL", "1")
	t.Setenv("ELEVENLABS_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "or")
	a, b := FromEnv()
	if s, ok := a.(*OpenRouterScribe); !ok || s.Key != "or" {
		t.Errorf("transcripts: %T", a)
	}
	if c, ok := b.(*OpenRouterChat); !ok || c.Key != "or" {
		t.Errorf("summaries: %T", b)
	}
	t.Setenv("ELEVENLABS_API_KEY", "el")
	t.Setenv("ANTHROPIC_API_KEY", "an")
	a, b = FromEnv()
	if _, ok := a.(*Scribe); !ok {
		t.Errorf("transcripts with ElevenLabs' key: %T", a)
	}
	if _, ok := b.(*Claude); !ok {
		t.Errorf("summaries with Anthropic's key: %T", b)
	}
}
