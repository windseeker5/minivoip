package notes

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"minivoip/store"
)

// The demo's services, with MINIVOIP_NOTES_LOCAL=1 and no keys: transcripts on this
// machine's graphics card, summaries through Ken's Claude Code login. His Max plan
// isn't for serving customers, so paying accounts need the keys.

//go:embed whisper.py
var whisperScript string

// Long enough for a long meeting, on the processor if the card can't.
const whisperTimeout = 2 * time.Hour

// Whisper writes down a recording with faster-whisper. It can't tell voices apart:
// channels still name who is who on a call, and all of a meeting is one speaker.
type Whisper struct {
	Program string // uv unless a test sets it
}

func (w *Whisper) Transcribe(ctx context.Context, file, contentType string, channels []string) (Transcript, error) {
	path, err := filepath.Abs(file)
	if err != nil {
		return Transcript{}, err
	}
	out, err := runLocal(ctx, whisperTimeout, "", orDefault(w.Program, "uv"),
		"run", "-q", "--no-project", "--with", "faster-whisper", "--with", "av<16",
		"--with", "nvidia-cublas-cu12", "--with", "nvidia-cudnn-cu12",
		"python", "-c", whisperScript, path, strconv.Itoa(len(channels)))
	if err != nil {
		return Transcript{}, fmt.Errorf("whisper: %w", err)
	}
	var a whisperAnswer
	if err := json.Unmarshal(out, &a); err != nil {
		return Transcript{}, fmt.Errorf("whisper: answer: %w", err)
	}
	return a.transcript(channels), nil
}

type whisperAnswer struct {
	Seconds float64 `json:"seconds"`
	Lines   []struct {
		Channel int     `json:"channel"`
		Start   float64 `json:"start"`
		Text    string  `json:"text"`
	} `json:"lines"`
}

// transcript puts the channels together, a new line each time someone else speaks.
func (a whisperAnswer) transcript(channels []string) Transcript {
	sort.SliceStable(a.Lines, func(i, j int) bool { return a.Lines[i].Start < a.Lines[j].Start })
	t := Transcript{Seconds: int64(math.Ceil(a.Seconds))}
	for _, l := range a.Lines {
		speaker := "speaker_0"
		if l.Channel >= 0 && l.Channel < len(channels) {
			speaker = channels[l.Channel]
		}
		if n := len(t.Lines); n > 0 && t.Lines[n-1].Speaker == speaker {
			t.Lines[n-1].Text += " " + l.Text
			continue
		}
		t.Lines = append(t.Lines, store.NoteLine{Speaker: speaker, Start: l.Start, Text: l.Text})
	}
	return t
}

// ClaudeCode writes the note with the claude command. Like Claude it gets nothing
// but the one transcript: no tools, no settings, no memory, and nothing it reads or
// keeps on this machine. The transcript goes in by its input, never on the command line.
type ClaudeCode struct {
	Program string // claude unless a test sets it
}

func (c *ClaudeCode) Summarize(ctx context.Context, lines []store.NoteLine, language store.Language, people []string) (Summary, error) {
	message, err := noteMessage(lines, people)
	if err != nil {
		return Summary{}, err
	}
	schema, err := json.Marshal(noteSchema)
	if err != nil {
		return Summary{}, err
	}
	out, err := runLocal(ctx, claudeTimeout, message, orDefault(c.Program, "claude"),
		"-p", "--model", claudeModel, "--effort", "medium", "--tools", "",
		"--strict-mcp-config", "--setting-sources", "", "--disable-slash-commands", "--no-session-persistence",
		"--system-prompt", notePrompt(language), "--json-schema", string(schema), "--output-format", "json")
	// A refused or used-up plan still answers with what went wrong.
	if err != nil && len(out) == 0 {
		return Summary{}, fmt.Errorf("claude code: %w", err)
	}
	return readClaudeCodeAnswer(out)
}

func readClaudeCodeAnswer(out []byte) (Summary, error) {
	var a struct {
		Subtype    string `json:"subtype"`
		IsError    bool   `json:"is_error"`
		StopReason string `json:"stop_reason"`
		Result     string `json:"result"`
		Note       *struct {
			Summary     string   `json:"summary"`
			NextActions []string `json:"next_actions"`
		} `json:"structured_output"`
	}
	if err := json.Unmarshal(out, &a); err != nil {
		return Summary{}, fmt.Errorf("claude code: answer: %w", err)
	}
	if a.StopReason == "refusal" {
		return Summary{}, fmt.Errorf("%w: claude code: stopped by refusal", ErrPermanent)
	}
	// Only an error's words reach the log, and kept short: a good answer holds the note.
	if a.IsError {
		return Summary{}, fmt.Errorf("claude code: %s: %s", a.Subtype, short(a.Result))
	}
	if a.Note == nil || strings.TrimSpace(a.Note.Summary) == "" {
		return Summary{}, errors.New("claude code: an empty summary")
	}
	return Summary{Text: strings.TrimSpace(a.Note.Summary), NextActions: a.Note.NextActions}, nil
}

// runLocal runs a command in an empty folder, with only what it needs from our
// environment, and gives back the last line it printed. The stop takes all of it
// down, so nothing is left holding the graphics card.
func runLocal(ctx context.Context, timeout time.Duration, input, program string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "minivoip-notes-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Env = localEnv()
	cmd.Stdin = strings.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("%w: %s", err, short(stderr.String()))
	}
	return lastLine(stdout.Bytes()), err
}

// localEnv holds no secrets of ours, and no API key: the plan's login is what pays.
func localEnv() []string {
	var env []string
	for _, name := range []string{"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TMPDIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "CLAUDE_CONFIG_DIR", "UV_CACHE_DIR", "HF_HOME", "HF_HUB_CACHE", "LD_LIBRARY_PATH"} {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// lastLine skips what a wrapper around the command may have printed first.
func lastLine(out []byte) []byte {
	out = bytes.TrimSpace(out)
	return out[bytes.LastIndexByte(out, '\n')+1:]
}

// short keeps what a command said to the end of it, where errors are, for the log.
func short(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}

func orDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}
