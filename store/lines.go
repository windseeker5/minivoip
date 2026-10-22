package store

import (
	"database/sql"
	"errors"
	"strings"

	"minivoip/lines"
)

var ErrNameTaken = errors.New("this line name is already taken")

// CreateLine gives an account its line, with a starter menu: a greeting
// and "1: talk to <owner>". The owner edits both in the menu editor.
func (s *Store) CreateLine(accountID int64, name, business, owner string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taken int
	if err := tx.QueryRow("SELECT count(*) FROM line WHERE name = ?", name).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return ErrNameTaken
	}
	res, err := tx.Exec("INSERT INTO line (account_id, name, business, owner_name, greeting) VALUES (?, ?, ?, ?, ?)",
		accountID, name, business, owner, "Hi, you've reached "+business+".")
	if err != nil {
		return err
	}
	lineID, _ := res.LastInsertId()
	if _, err := tx.Exec("INSERT INTO menu_option (line_id, position, key, label, action, say) VALUES (?, 0, '1', ?, 'ring', ?)",
		lineID, "Talk to "+owner, "Please hold, I'm ringing "+owner+"."); err != nil {
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

func (s *Store) line(where string, arg any) (lines.Line, error) {
	var l lines.Line
	var id int64
	err := s.db.QueryRow("SELECT id, name, business, owner_name, greeting FROM line WHERE "+where, arg).
		Scan(&id, &l.Name, &l.Business, &l.Owner, &l.Greeting)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	rows, err := s.db.Query("SELECT key, label, action, say, url FROM menu_option WHERE line_id = ? ORDER BY position", id)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var o lines.Option
		if err := rows.Scan(&o.Key, &o.Label, &o.Action, &o.Say, &o.URL); err != nil {
			return l, err
		}
		l.Options = append(l.Options, o)
	}
	return l, rows.Err()
}

// SaveMenu replaces the account's line details, greeting and menu options.
func (s *Store) SaveMenu(accountID int64, business, owner, greeting string, opts []lines.Option) error {
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
	if _, err := tx.Exec("UPDATE line SET business = ?, owner_name = ?, greeting = ? WHERE id = ?",
		strings.TrimSpace(business), strings.TrimSpace(owner), strings.TrimSpace(greeting), lineID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM menu_option WHERE line_id = ?", lineID); err != nil {
		return err
	}
	for i, o := range opts {
		if _, err := tx.Exec("INSERT INTO menu_option (line_id, position, key, label, action, say, url) VALUES (?, ?, ?, ?, ?, ?, ?)",
			lineID, i, o.Key, o.Label, string(o.Action), o.Say, o.URL); err != nil {
			return err
		}
	}
	return tx.Commit()
}
