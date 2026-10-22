package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Prompts are the owner's recorded messages (greeting, choice messages).
// They're public: callers' browsers fetch them at /prompt/<name>. Names are
// random, so they can't be guessed.

// MaxPromptBytes caps one recording (a minute of Opus is about 500 KB).
const MaxPromptBytes = 3 << 20

var promptName = regexp.MustCompile(`^prompt-[0-9a-f]{32}\.(webm|m4a|ogg|mp3)$`)

func (s *Store) promptDir() string { return filepath.Join(s.Dir, "prompts") }

// PromptPath is the file for a prompt name ("" if the name isn't valid).
func (s *Store) PromptPath(name string) string {
	if !promptName.MatchString(name) {
		return ""
	}
	return filepath.Join(s.promptDir(), name)
}

// PromptExists says whether a prompt name is valid and its file is there.
func (s *Store) PromptExists(name string) bool {
	p := s.PromptPath(name)
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// AddPrompt saves a recording and returns its name.
func (s *Store) AddPrompt(contentType string, audio io.Reader) (string, error) {
	if err := os.MkdirAll(s.promptDir(), 0o755); err != nil {
		return "", err
	}
	b := make([]byte, 16)
	rand.Read(b)
	name := "prompt-" + hex.EncodeToString(b) + extFor(contentType)
	path := filepath.Join(s.promptDir(), name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(audio, MaxPromptBytes+1))
	f.Close()
	if err == nil && (n == 0 || n > MaxPromptBytes) {
		err = errors.New("recording empty or too long")
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return name, nil
}
