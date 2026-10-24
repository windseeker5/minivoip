// Package store keeps accounts, users, sessions, lines and menus in one
// SQLite file. Tables and columns are described in docs/data-dictionary.md;
// change both together.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
	// Dir is the folder holding the database; audio files go under it.
	Dir string
}

// migrations run in order; PRAGMA user_version remembers how many ran.
// Never edit one that has shipped: add a new one.
var migrations = []string{
	// 1: accounts, users, sessions, lines, menu options, promo codes.
	`
	CREATE TABLE account (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL,
		promo_code  TEXT REFERENCES promo_code(code),
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE user (
		id            INTEGER PRIMARY KEY,
		account_id    INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
		email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		created_at    TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE session (
		token       TEXT PRIMARY KEY,
		user_id     INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
		expires_at  TEXT NOT NULL
	);
	CREATE TABLE line (
		id          INTEGER PRIMARY KEY,
		account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
		name        TEXT NOT NULL UNIQUE,
		business    TEXT NOT NULL,
		owner_name  TEXT NOT NULL,
		greeting    TEXT NOT NULL,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE menu_option (
		id        INTEGER PRIMARY KEY,
		line_id   INTEGER NOT NULL REFERENCES line(id) ON DELETE CASCADE,
		position  INTEGER NOT NULL,
		key       TEXT NOT NULL,
		label     TEXT NOT NULL,
		action    TEXT NOT NULL CHECK (action IN ('ring', 'play', 'link', 'voicemail')),
		say       TEXT NOT NULL DEFAULT '',
		url       TEXT NOT NULL DEFAULT '',
		UNIQUE (line_id, key)
	);
	CREATE TABLE promo_code (
		code        TEXT PRIMARY KEY COLLATE NOCASE,
		valid_from  TEXT,
		valid_to    TEXT,
		max_uses    INTEGER NOT NULL DEFAULT 0,
		times_used  INTEGER NOT NULL DEFAULT 0,
		revoked     INTEGER NOT NULL DEFAULT 0,
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	);
	`,
	// 2: voicemail messages (audio files live in <data dir>/voicemail/).
	`
	CREATE TABLE voicemail (
		id           INTEGER PRIMARY KEY,
		line_id      INTEGER NOT NULL REFERENCES line(id) ON DELETE CASCADE,
		option       TEXT NOT NULL DEFAULT '',
		duration_ms  INTEGER NOT NULL DEFAULT 0,
		file         TEXT NOT NULL,
		content_type TEXT NOT NULL,
		listened     INTEGER NOT NULL DEFAULT 0,
		created_at   TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX voicemail_line ON voicemail(line_id, created_at);
	`,
	// 3: recorded prompts (files live in <data dir>/prompts/).
	`
	ALTER TABLE line ADD COLUMN greeting_audio TEXT NOT NULL DEFAULT '';
	ALTER TABLE menu_option ADD COLUMN say_audio TEXT NOT NULL DEFAULT '';
	`,
	// 4: phones that get push notifications, and server settings (VAPID keys).
	`
	CREATE TABLE device (
		id          INTEGER PRIMARY KEY,
		account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
		endpoint    TEXT NOT NULL UNIQUE,
		p256dh      TEXT NOT NULL,
		auth        TEXT NOT NULL,
		user_agent  TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE setting (
		key    TEXT PRIMARY KEY,
		value  TEXT NOT NULL
	);
	`,
	// 5: admins (promo code page).
	`
	ALTER TABLE user ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;
	`,
	// 6: delivery receipts for push notifications (kept 14 days).
	`
	CREATE TABLE push_log (
		id         INTEGER PRIMARY KEY,
		device_id  INTEGER NOT NULL,
		line       TEXT NOT NULL,
		call_id    TEXT NOT NULL DEFAULT '',
		kind       TEXT NOT NULL,
		sent_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now')),
		result     TEXT NOT NULL DEFAULT '',
		shown_at   TEXT,
		shown_as   TEXT NOT NULL DEFAULT '',
		phone_note TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX push_log_sent ON push_log(sent_at);
	`,
	// 7: call history, voicemails tied to their call, and the owner's
	// Available switch.
	`
	CREATE TABLE call_log (
		id          INTEGER PRIMARY KEY,
		line_id     INTEGER NOT NULL REFERENCES line(id) ON DELETE CASCADE,
		call_id     TEXT NOT NULL UNIQUE,
		option      TEXT NOT NULL DEFAULT '',
		outcome     TEXT NOT NULL CHECK (outcome IN ('answered', 'missed', 'declined', 'away')),
		started_at  TEXT NOT NULL,
		answered_at TEXT,
		ended_at    TEXT NOT NULL
	);
	CREATE INDEX call_log_line ON call_log(line_id, started_at);
	ALTER TABLE voicemail ADD COLUMN call_id TEXT NOT NULL DEFAULT '';
	ALTER TABLE line ADD COLUMN away_until TEXT NOT NULL DEFAULT '';
	`,
}

// Open opens (and creates or upgrades) the database file at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, Dir: filepath.Dir(path)}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
