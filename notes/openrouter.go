package notes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"minivoip/store"
)

// OpenRouter is one key for both services: ElevenLabs Scribe behind its speech to
// text, a Claude model behind its chat. Ken's only key (2026-11-21). Neither request
// lets a provider keep or learn from what callers said (data_collection: deny).
const (
	openRouterURL         = "https://openrouter.ai/api/v1"
	openRouterScribeModel = "elevenlabs/scribe-v2"
	openRouterNoteModel   = "anthropic/claude-sonnet-5.5"
)

// OpenRouterScribe writes a recording down through OpenRouter. OpenRouter doesn't
// pass on Scribe's channel option, so a call's two channels are cut apart with
// ffmpeg and each is written down on its own, which bills the same as Scribe's
// channels. Without ffmpeg, the service tells the voices apart instead.
type OpenRouterScribe struct {
	Key    string
	URL    string // the real service unless a test sets it
	FFmpeg string // ffmpeg unless a test sets it
}

type openRouterWords struct {
	Duration *float64 `json:"duration"`
	Usage    struct {
		Seconds *float64 `json:"seconds"`
	} `json:"usage"`
	Text  string `json:"text"`
	Words []struct {
		Word         string   `json:"word"`
		Type         string   `json:"type"` // word or audio_event
		Start        *float64 `json:"start"`
		End          *float64 `json:"end"`
		Speaker      *int     `json:"speaker"`
		SpeakerLabel string   `json:"speaker_label"`
	} `json:"words"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenRouterScribe) Transcribe(ctx context.Context, file, contentType string, channels []string) (Transcript, error) {
	ctx, cancel := context.WithTimeout(ctx, scribeTimeout)
	defer cancel()
	ffmpeg := orDefault(c.FFmpeg, "ffmpeg")
	if _, err := exec.LookPath(ffmpeg); err != nil || len(channels) == 0 {
		a, err := c.send(ctx, file, audioFormat(contentType), true)
		if err != nil {
			return Transcript{}, err
		}
		return scribeAnswer{AudioSeconds: a.seconds(), Transcripts: []scribeChannel{{Words: a.words(nil)}}}.transcript(nil), nil
	}

	dir, err := os.MkdirTemp("", "minivoip-channels-")
	if err != nil {
		return Transcript{}, err
	}
	defer os.RemoveAll(dir)
	path, err := filepath.Abs(file)
	if err != nil {
		return Transcript{}, err
	}
	var all scribeAnswer
	var seconds float64
	for i := range channels {
		// One voice, at the bitrate of a voice: an hour is about 14 MB.
		part := filepath.Join(dir, fmt.Sprintf("channel-%d.webm", i))
		if _, err := runLocal(ctx, scribeTimeout, "", ffmpeg, "-nostdin", "-loglevel", "error", "-i", path,
			"-af", fmt.Sprintf("pan=mono|c0=c%d", i), "-c:a", "libopus", "-b:a", "32k", part); err != nil {
			if ctx.Err() == nil {
				err = fmt.Errorf("%w: %w", ErrPermanent, err) // the recording can't be read
			}
			return Transcript{}, fmt.Errorf("ffmpeg: %w", err)
		}
		a, err := c.send(ctx, part, "webm", false)
		if err != nil {
			return Transcript{}, err
		}
		if s := a.seconds(); s != nil {
			seconds += *s
		}
		ch := i
		all.Transcripts = append(all.Transcripts, scribeChannel{Channel: &ch, Words: a.words(&ch)})
	}
	all.AudioSeconds = &seconds
	return all.transcript(channels), nil
}

// send streams the recording as base64 in a JSON body, the way OpenRouter takes
// files over 25 MB, so a long meeting needn't be cut up.
func (c *OpenRouterScribe) send(ctx context.Context, file, format string, diarize bool) (openRouterWords, error) {
	f, err := os.Open(file)
	if err != nil {
		return openRouterWords{}, err
	}
	defer f.Close()
	fields := map[string]any{
		"model":                   openRouterScribeModel,
		"response_format":         "verbose_json",
		"timestamp_granularities": []string{"word"},
		"provider":                map[string]any{"data_collection": "deny"},
	}
	if diarize {
		fields["diarize"] = true
	}
	head, err := json.Marshal(fields)
	if err != nil {
		return openRouterWords{}, err
	}
	formatJSON, err := json.Marshal(format)
	if err != nil {
		return openRouterWords{}, err
	}
	body, send := io.Pipe()
	go func() {
		send.CloseWithError(func() error {
			if _, err := fmt.Fprintf(send, `%s,"input_audio":{"format":%s,"data":"`, head[:len(head)-1], formatJSON); err != nil {
				return err
			}
			enc := base64.NewEncoder(base64.StdEncoding, send)
			if _, err := io.Copy(enc, f); err != nil {
				return err
			}
			if err := enc.Close(); err != nil {
				return err
			}
			_, err := io.WriteString(send, `"}}`)
			return err
		}())
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.URL, openRouterURL)+"/audio/transcriptions", body)
	if err != nil {
		body.Close()
		return openRouterWords{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return openRouterWords{}, fmt.Errorf("openrouter scribe: %w", err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(resp.Body, maxScribeAnswer))
	if err != nil {
		return openRouterWords{}, fmt.Errorf("openrouter scribe: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return openRouterWords{}, statusError("openrouter scribe", resp.StatusCode, answer)
	}
	var a openRouterWords
	if err := json.Unmarshal(answer, &a); err != nil {
		return openRouterWords{}, fmt.Errorf("openrouter scribe: answer: %w", err)
	}
	if a.Error != nil {
		return openRouterWords{}, fmt.Errorf("openrouter scribe: %s", short(a.Error.Message))
	}
	return a, nil
}

func (a openRouterWords) seconds() *float64 {
	if a.Duration != nil {
		return a.Duration
	}
	return a.Usage.Seconds
}

// words turns OpenRouter's words into Scribe's, so one transcript() puts them together.
// A model without word times gives its text as one line.
func (a openRouterWords) words(channel *int) []scribeWord {
	var words []scribeWord
	for _, w := range a.Words {
		kind := "word"
		if w.Type != "" && w.Type != "word" {
			kind = w.Type
		}
		speaker := w.SpeakerLabel
		if speaker == "" && w.Speaker != nil {
			speaker = fmt.Sprintf("speaker_%d", *w.Speaker)
		}
		if speaker == "" {
			speaker = "speaker_0"
		}
		words = append(words, scribeWord{Text: w.Word, Type: kind, Start: w.Start, End: w.End, Speaker: speaker, Channel: channel})
	}
	if len(words) == 0 && strings.TrimSpace(a.Text) != "" {
		zero := 0.0
		words = append(words, scribeWord{Text: strings.TrimSpace(a.Text), Type: "word", Start: &zero, Speaker: "speaker_0", Channel: channel})
	}
	return words
}

// audioFormat is what OpenRouter calls a recording's type (store.AudioType).
func audioFormat(contentType string) string {
	switch t, _, _ := strings.Cut(contentType, ";"); strings.TrimSpace(t) {
	case "audio/mp4":
		return "m4a"
	case "audio/mpeg":
		return "mp3"
	case "audio/ogg":
		return "ogg"
	default:
		return "webm"
	}
}

// OpenRouterChat writes the note with a Claude model through OpenRouter. Like
// Claude, it gets nothing but the one transcript and no tools, and its answer must
// fit the schema; providers that can't keep to the schema aren't used.
type OpenRouterChat struct {
	Key string
	URL string // the real service unless a test sets it
}

func (c *OpenRouterChat) Summarize(ctx context.Context, lines []store.NoteLine, language store.Language, people []string) (Summary, error) {
	message, err := noteMessage(lines, people)
	if err != nil {
		return Summary{}, err
	}
	body, err := json.Marshal(map[string]any{
		"model":      openRouterNoteModel,
		"max_tokens": 16000,
		"messages": []map[string]any{
			{"role": "system", "content": notePrompt(language)},
			{"role": "user", "content": message},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "note", "strict": true, "schema": noteSchema},
		},
		"provider": map[string]any{"data_collection": "deny", "require_parameters": true},
	})
	if err != nil {
		return Summary{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, claudeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.URL, openRouterURL)+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Summary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return Summary{}, fmt.Errorf("openrouter chat: %w", err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Summary{}, fmt.Errorf("openrouter chat: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Summary{}, statusError("openrouter chat", resp.StatusCode, answer)
	}
	return readOpenRouterAnswer(answer)
}

func readOpenRouterAnswer(answer []byte) (Summary, error) {
	var a struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(answer, &a); err != nil {
		return Summary{}, fmt.Errorf("openrouter chat: answer: %w", err)
	}
	if a.Error != nil {
		return Summary{}, fmt.Errorf("openrouter chat: %s", short(a.Error.Message))
	}
	if len(a.Choices) == 0 {
		return Summary{}, errors.New("openrouter chat: no answer")
	}
	choice := a.Choices[0]
	// Checked before the content: the same transcript would stop the same way.
	if choice.Message.Refusal != "" || choice.FinishReason == "content_filter" || choice.FinishReason == "length" {
		return Summary{}, fmt.Errorf("%w: openrouter chat: stopped by %s", ErrPermanent, orDefault(choice.FinishReason, "refusal"))
	}
	return parseNote("openrouter chat", choice.Message.Content)
}
