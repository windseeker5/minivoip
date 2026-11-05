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

// Prompts are the owner's recorded or uploaded messages (greeting, choice messages).
// They're public: callers' browsers fetch them at /prompt/<name>. Names are
// random, so they can't be guessed. Each belongs to the account that recorded
// it (table prompt): nobody else can use it in a menu, or delete it by
// dropping it from one.

// MaxPromptBytes caps one recording (a minute of Opus is about 500 KB).
const MaxPromptBytes = 3 << 20

// MaxPrompts caps an account's recordings. A menu uses 11 at most; recordings
// no menu uses go away after a day, so re-recording never hits this.
const MaxPrompts = 40

var ErrTooManyPrompts = errors.New("too many recordings")

var promptName = regexp.MustCompile(`^prompt-[0-9a-f]{32}\.(webm|m4a|ogg|mp3)$`)

func (s *Store) promptDir() string { return filepath.Join(s.Dir, "prompts") }

// PromptPath is the file for a prompt name ("" if the name isn't valid).
func (s *Store) PromptPath(name string) string {
	if !promptName.MatchString(name) {
		return ""
	}
	return filepath.Join(s.promptDir(), name)
}

// PromptType is what a recording is served as, by its name.
func PromptType(name string) string {
	switch filepath.Ext(name) {
	case ".m4a":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	}
	return "audio/webm"
}

// OwnsPrompt says whether a recording belongs to the account and its file is
// there.
func (s *Store) OwnsPrompt(accountID int64, name string) bool {
	p := s.PromptPath(name)
	if p == "" {
		return false
	}
	var n int
	s.db.QueryRow("SELECT count(*) FROM prompt WHERE name = ? AND account_id = ?", name, accountID).Scan(&n)
	if n == 0 {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// removePrompt deletes one of the account's recordings; someone else's stays.
func (s *Store) removePrompt(accountID int64, name string) {
	res, err := s.db.Exec("DELETE FROM prompt WHERE name = ? AND account_id = ?", name, accountID)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		os.Remove(s.PromptPath(name))
	}
}

// sweepPrompts deletes the account's recordings no menu uses, a day old or
// more (recorded and never saved).
func (s *Store) sweepPrompts(accountID int64) error {
	rows, err := s.db.Query(`SELECT name FROM prompt WHERE account_id = ?1 AND created_at < datetime('now', '-1 day')
		AND name NOT IN (SELECT greeting_audio FROM line WHERE account_id = ?1)
		AND name NOT IN (SELECT voicemail_audio FROM line WHERE account_id = ?1)
		AND name NOT IN (SELECT o.say_audio FROM menu_option o JOIN line l ON l.id = o.line_id WHERE l.account_id = ?1)`, accountID)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, n := range names {
		s.removePrompt(accountID, n)
	}
	return nil
}

// AddPrompt saves a recording for the account and returns its name.
func (s *Store) AddPrompt(accountID int64, contentType string, audio io.Reader) (string, error) {
	if err := s.sweepPrompts(accountID); err != nil {
		return "", err
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM prompt WHERE account_id = ?", accountID).Scan(&count); err != nil {
		return "", err
	}
	if count >= MaxPrompts {
		return "", ErrTooManyPrompts
	}
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
	if err == nil {
		_, err = s.db.Exec("INSERT INTO prompt (name, account_id) VALUES (?, ?)", name, accountID)
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return name, nil
}
