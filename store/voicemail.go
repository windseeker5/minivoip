package store

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Voicemail struct {
	ID          int64
	CallID      string // the unanswered call it follows; "" for a voicemail choice
	Option      string // menu choice the caller picked, if any
	DurationMS  int64
	ContentType string
	Listened    bool
	CreatedAt   string // UTC "YYYY-MM-DD HH:MM:SS"
}

// Seconds is the message length, rounded up.
func (v Voicemail) Seconds() int64 { return (v.DurationMS + 999) / 1000 }

// MaxVoicemailBytes caps one message (two minutes of Opus is about 1 MB).
const MaxVoicemailBytes = 8 << 20

func (s *Store) voicemailDir() string { return filepath.Join(s.Dir, "voicemail") }

// extFor picks a file extension for a recording's content type.
func extFor(contentType string) string {
	switch {
	case strings.Contains(contentType, "mp4"), strings.Contains(contentType, "aac"):
		return ".m4a"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	case strings.Contains(contentType, "mpeg"):
		return ".mp3"
	default:
		return ".webm"
	}
}

// AddVoicemail saves a caller's message for a line. callID is the
// unanswered call it follows, or "".
func (s *Store) AddVoicemail(lineName, option, callID, contentType string, durationMS int64, audio io.Reader) (Voicemail, error) {
	var lineID int64
	err := s.db.QueryRow("SELECT id FROM line WHERE name = ?", lineName).Scan(&lineID)
	if errors.Is(err, sql.ErrNoRows) {
		return Voicemail{}, ErrNotFound
	}
	if err != nil {
		return Voicemail{}, err
	}
	if err := os.MkdirAll(s.voicemailDir(), 0o755); err != nil {
		return Voicemail{}, err
	}
	f, err := os.CreateTemp(s.voicemailDir(), "msg-*"+extFor(contentType))
	if err != nil {
		return Voicemail{}, err
	}
	n, err := io.Copy(f, io.LimitReader(audio, MaxVoicemailBytes+1))
	f.Close()
	if err == nil && n > MaxVoicemailBytes {
		err = fmt.Errorf("voicemail too large")
	}
	if err == nil && n == 0 {
		err = fmt.Errorf("empty voicemail")
	}
	if err != nil {
		os.Remove(f.Name())
		return Voicemail{}, err
	}
	res, err := s.db.Exec("INSERT INTO voicemail (line_id, option, call_id, duration_ms, file, content_type) VALUES (?, ?, ?, ?, ?, ?)",
		lineID, option, callID, durationMS, filepath.Base(f.Name()), contentType)
	if err != nil {
		os.Remove(f.Name())
		return Voicemail{}, err
	}
	id, _ := res.LastInsertId()
	return Voicemail{ID: id, CallID: callID, Option: option, DurationMS: durationMS, ContentType: contentType}, nil
}

// Voicemails lists an account's messages, newest first.
func (s *Store) Voicemails(accountID int64) ([]Voicemail, error) {
	rows, err := s.db.Query(`
		SELECT v.id, v.call_id, v.option, v.duration_ms, v.content_type, v.listened, v.created_at
		FROM voicemail v JOIN line l ON l.id = v.line_id
		WHERE l.account_id = ? ORDER BY v.created_at DESC, v.id DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Voicemail
	for rows.Next() {
		var v Voicemail
		if err := rows.Scan(&v.ID, &v.CallID, &v.Option, &v.DurationMS, &v.ContentType, &v.Listened, &v.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

// NewVoicemails counts an account's messages not yet played.
func (s *Store) NewVoicemails(accountID int64) int {
	var n int
	s.db.QueryRow(`SELECT count(*) FROM voicemail v JOIN line l ON l.id = v.line_id
		WHERE l.account_id = ? AND v.listened = 0`, accountID).Scan(&n)
	return n
}

// VoicemailFile returns the path and content type of one of the account's
// messages, or ErrNotFound.
func (s *Store) VoicemailFile(accountID, id int64) (string, string, error) {
	var file, ct string
	err := s.db.QueryRow(`SELECT v.file, v.content_type FROM voicemail v JOIN line l ON l.id = v.line_id
		WHERE v.id = ? AND l.account_id = ?`, id, accountID).Scan(&file, &ct)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return filepath.Join(s.voicemailDir(), file), ct, err
}

// MarkListened records that the owner played a message.
func (s *Store) MarkListened(accountID, id int64) error {
	_, err := s.db.Exec(`UPDATE voicemail SET listened = 1 WHERE id = ?
		AND line_id IN (SELECT id FROM line WHERE account_id = ?)`, id, accountID)
	return err
}

// DeleteVoicemail removes a message and its audio file.
func (s *Store) DeleteVoicemail(accountID, id int64) error {
	path, _, err := s.VoicemailFile(accountID, id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec("DELETE FROM voicemail WHERE id = ?", id); err != nil {
		return err
	}
	os.Remove(path)
	return nil
}
