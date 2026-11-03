package store

import (
	"database/sql"
	"errors"
	"strings"

	"minivoip/lines"
)

var (
	ErrNameTaken = errors.New("this line name is already taken")
	ErrHasLine   = errors.New("this account already has a line")
)

// CreateLine gives an account its line, with a starter menu: a greeting
// and "1: talk to us". The owner edits both in the menu editor. owner_name
// holds the business name: callers hear "Ringing Joe's B&B". One line per
// account.
func (s *Store) CreateLine(accountID int64, name, business string) error {
	owner := business
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var has int
	if err := tx.QueryRow("SELECT count(*) FROM line WHERE account_id = ?", accountID).Scan(&has); err != nil {
		return err
	}
	if has > 0 {
		return ErrHasLine
	}
	var taken int
	if err := tx.QueryRow("SELECT count(*) FROM line WHERE name = ?", name).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return ErrNameTaken
	}
	res, err := tx.Exec("INSERT INTO line (account_id, name, business, owner_name, greeting) VALUES (?, ?, ?, ?, ?)",
		accountID, name, business, owner, lines.StarterGreeting(business))
	if err != nil {
		return err
	}
	lineID, _ := res.LastInsertId()
	if _, err := tx.Exec("INSERT INTO menu_option (line_id, position, key, label, action, say) VALUES (?, 0, '1', ?, 'ring', ?)",
		lineID, lines.StarterChoice, lines.StarterSay); err != nil {
		return err
	}
	return tx.Commit()
}

// LineByName returns a line and its menu, or ErrNotFound.
func (s *Store) LineByName(name string) (lines.Line, error) {
	return s.line("name = ?", name)
}

// LineForAccount returns the account's line, or ErrNotFound.
func (s *Store) LineForAccount(accountID int64) (lines.Line, error) {
	return s.line("account_id = ?", accountID)
}

func (s *Store) LineExists(name string) bool {
	var n int
	s.db.QueryRow("SELECT count(*) FROM line WHERE name = ?", name).Scan(&n)
	return n > 0
}

// HasRoom says whether a line has a meeting room: its account is Premium.
func (s *Store) HasRoom(name string) bool {
	var n int
	s.db.QueryRow("SELECT count(*) FROM line l JOIN account a ON a.id = l.account_id WHERE l.name = ? AND a.plan = 'premium'", name).Scan(&n)
	return n > 0
}

func (s *Store) line(where string, arg any) (lines.Line, error) {
	var l lines.Line
	var id int64
	err := s.db.QueryRow(`SELECT id, name, business, owner_name, greeting, greeting_audio, voicemail_text, voicemail_audio, away_until, logo,
		(SELECT plan FROM account WHERE account.id = line.account_id) = 'premium' FROM line WHERE `+where, arg).
		Scan(&id, &l.Name, &l.Business, &l.Owner, &l.Greeting, &l.GreetingAudio, &l.VoicemailText, &l.VoicemailAudio, &l.AwayUntil, &l.Logo, &l.Premium)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	rows, err := s.db.Query("SELECT key, label, action, say, url, say_audio FROM menu_option WHERE line_id = ? ORDER BY position", id)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var o lines.Option
		if err := rows.Scan(&o.Key, &o.Label, &o.Action, &o.Say, &o.URL, &o.SayAudio); err != nil {
			return l, err
		}
		l.Options = append(l.Options, o)
	}
	return l, rows.Err()
}

// SaveMenu replaces the account's line details, greetings and menu options.
// Recordings the menu no longer uses are deleted.
func (s *Store) SaveMenu(accountID int64, l lines.Line) error {
	old, err := s.LineForAccount(accountID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lineID int64
	err = tx.QueryRow("SELECT id FROM line WHERE account_id = ?", accountID).Scan(&lineID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE line SET business = ?, owner_name = ?, greeting = ?, greeting_audio = ?, voicemail_text = ?, voicemail_audio = ? WHERE id = ?",
		strings.TrimSpace(l.Business), strings.TrimSpace(l.Owner), strings.TrimSpace(l.Greeting), l.GreetingAudio, strings.TrimSpace(l.VoicemailText), l.VoicemailAudio, lineID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM menu_option WHERE line_id = ?", lineID); err != nil {
		return err
	}
	for i, o := range l.Options {
		if _, err := tx.Exec("INSERT INTO menu_option (line_id, position, key, label, action, say, url, say_audio) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			lineID, i, o.Key, o.Label, string(o.Action), o.Say, o.URL, o.SayAudio); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	keep := map[string]bool{l.GreetingAudio: true, l.VoicemailAudio: true}
	for _, o := range l.Options {
		keep[o.SayAudio] = true
	}
	for _, f := range append([]string{old.GreetingAudio, old.VoicemailAudio}, sayAudios(old.Options)...) {
		if f != "" && !keep[f] {
			s.removePrompt(accountID, f)
		}
	}
	return nil
}

func sayAudios(opts []lines.Option) []string {
	var out []string
	for _, o := range opts {
		out = append(out, o.SayAudio)
	}
	return out
}
