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

// MaxVoicemailBytes caps one message (two minutes of Opus is about 1 MB, of
// Safari's recordings about 2 MB).
const MaxVoicemailBytes = 4 << 20

// A mailbox holds this many messages or bytes; past that, callers are told to
// try later. All mailboxes together stop at MaxVoicemailDisk.
const (
	MaxMailbox       = 300
	MaxMailboxBytes  = 400 << 20
	MaxVoicemailDisk = 10 << 30
)

// MailboxFull says whether a line can't take another message. It runs with the
// call hub locked, so it asks the database and never reads the disk.
func (s *Store) MailboxFull(line string) bool {
	var n int
	var size, total int64
	s.db.QueryRow("SELECT count(*), COALESCE(sum(v.size), 0) FROM voicemail v JOIN line l ON l.id = v.line_id WHERE l.name = ?", line).Scan(&n, &size)
	if n >= MaxMailbox || size >= MaxMailboxBytes {
		return true
	}
	s.db.QueryRow("SELECT COALESCE(sum(size), 0) FROM voicemail").Scan(&total)
	return total >= MaxVoicemailDisk
}

// sizeVoicemails fills in the size of messages saved before migration 13.
func (s *Store) sizeVoicemails() error {
	rows, err := s.db.Query("SELECT id, file FROM voicemail WHERE size = 0")
	if err != nil {
		return err
	}
	sizes := map[int64]int64{}
	for rows.Next() {
		var id int64
		var file string
		if err := rows.Scan(&id, &file); err != nil {
			rows.Close()
			return err
		}
		if info, err := os.Stat(filepath.Join(s.voicemailDir(), file)); err == nil {
			sizes[id] = info.Size()
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for id, size := range sizes {
		if _, err := s.db.Exec("UPDATE voicemail SET size = ? WHERE id = ?", size, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) voicemailDir() string { return filepath.Join(s.Dir, "voicemail") }

// AudioType is the recording type to keep for what a browser sent: one of the
// four kinds every browser plays, or "" for anything else. It is also what
// voicemails are served as, so a made-up type can never turn into a web page.
func AudioType(header string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(header, ";")[0])) {
	case "audio/webm", "video/webm": // Chrome's recorder can say video/webm
		return "audio/webm"
	case "audio/ogg":
		return "audio/ogg"
	case "audio/mp4", "audio/m4a", "audio/x-m4a", "audio/aac":
		return "audio/mp4"
	case "audio/mpeg", "audio/mp3":
		return "audio/mpeg"
	}
	return ""
}

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
	res, err := s.db.Exec("INSERT INTO voicemail (line_id, option, call_id, duration_ms, file, content_type, size) VALUES (?, ?, ?, ?, ?, ?, ?)",
		lineID, option, callID, durationMS, filepath.Base(f.Name()), contentType, n)
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

// Keep voicemails this long, and at most this many per line (the oldest heard
// ones go first; one not yet heard is never dropped for space); call history a
// year.
const (
	VoicemailDays   = 90
	VoicemailsKept  = 200
	callHistoryDays = 365
)

// Cleanup forgets expired logins, old voicemails with their audio, old call
// history and old notes. main runs it once a day.
func (s *Store) Cleanup() error {
	if _, err := s.db.Exec("DELETE FROM session WHERE expires_at <= datetime('now')"); err != nil {
		return err
	}
	if err := s.cleanupNotes(); err != nil {
		return err
	}
	if _, err := s.db.Exec("DELETE FROM call_log WHERE started_at < datetime('now', ?)", fmt.Sprintf("-%d days", callHistoryDays)); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT id, file FROM (
			SELECT id, file, created_at, listened, row_number() OVER (PARTITION BY line_id ORDER BY created_at DESC, id DESC) AS n FROM voicemail
		) WHERE (n > ? AND listened = 1) OR created_at < datetime('now', ?)`, VoicemailsKept, fmt.Sprintf("-%d days", VoicemailDays))
	if err != nil {
		return err
	}
	type old struct {
		id   int64
		file string
	}
	var list []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.id, &o.file); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, o := range list {
		if _, err := s.db.Exec("DELETE FROM voicemail WHERE id = ?", o.id); err != nil {
			return err
		}
		os.Remove(filepath.Join(s.voicemailDir(), o.file))
	}
	return nil
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
