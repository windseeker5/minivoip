package store

import (
	"database/sql"
	"errors"
	"time"
)

// The call log is the owner's call history: one row per call that reached
// the line (answered, missed, declined, or sent to voicemail while away).
// Voicemails point at the call they follow (voicemail.call_id).

// CallRecord is one finished call.
type CallRecord struct {
	CallID  string
	Option  string
	Outcome string // answered, missed, declined, away
	Started time.Time
	// Answered is zero when nobody answered.
	Answered time.Time
	Ended    time.Time
}

func utc(t time.Time) string { return t.UTC().Format(time.DateTime) }

// LogCall saves a finished call to its line's history.
func (s *Store) LogCall(lineName string, c CallRecord) error {
	var answered any
	if !c.Answered.IsZero() {
		answered = utc(c.Answered)
	}
	_, err := s.db.Exec(`INSERT INTO call_log (line_id, call_id, option, outcome, started_at, answered_at, ended_at)
		SELECT id, ?, ?, ?, ?, ?, ? FROM line WHERE name = ?`,
		c.CallID, c.Option, c.Outcome, utc(c.Started), answered, utc(c.Ended), lineName)
	return err
}

// HistoryEntry is one row of the phone app's call list: a call, or a
// voicemail left by picking a voicemail choice.
type HistoryEntry struct {
	// Outcome is answered, missed, declined, away, or "" for a voicemail
	// choice (no call rang).
	Outcome string
	Option  string
	At      string // UTC "YYYY-MM-DD HH:MM:SS"
	// TalkSeconds is how long an answered call lasted.
	TalkSeconds int64
	// Voicemail is the message the caller left, if any.
	Voicemail *Voicemail
}

// CallHistory lists an account's calls and voicemails, newest first.
func (s *Store) CallHistory(accountID int64, limit int) ([]HistoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT c.outcome, c.option, c.started_at,
			coalesce(CAST(strftime('%s', c.ended_at) - strftime('%s', c.answered_at) AS INTEGER), 0),
			v.id, v.call_id, v.option, v.duration_ms, v.content_type, v.listened, v.created_at
		FROM call_log c JOIN line l ON l.id = c.line_id
		LEFT JOIN voicemail v ON v.call_id = c.call_id
		WHERE l.account_id = ?
		UNION ALL
		SELECT '', v.option, v.created_at, 0,
			v.id, v.call_id, v.option, v.duration_ms, v.content_type, v.listened, v.created_at
		FROM voicemail v JOIN line l ON l.id = v.line_id
		WHERE l.account_id = ? AND (v.call_id = '' OR v.call_id NOT IN (SELECT call_id FROM call_log))
		ORDER BY 3 DESC
		LIMIT ?`, accountID, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var vid sql.NullInt64
		var v Voicemail
		var vCall, vOption, vType, vAt sql.NullString
		var vMS sql.NullInt64
		var vListened sql.NullBool
		if err := rows.Scan(&e.Outcome, &e.Option, &e.At, &e.TalkSeconds,
			&vid, &vCall, &vOption, &vMS, &vType, &vListened, &vAt); err != nil {
			return nil, err
		}
		if vid.Valid {
			v = Voicemail{ID: vid.Int64, CallID: vCall.String, Option: vOption.String, DurationMS: vMS.Int64,
				ContentType: vType.String, Listened: vListened.Bool, CreatedAt: vAt.String}
			e.Voicemail = &v
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

// Away is the "Off" setting of the Available switch: away until turned back on.
const Away = "9999-12-31 00:00:00"

// SetAwayUntil sets the account's Available switch: "" is available, a
// UTC time is away until then (Away: until turned back on).
func (s *Store) SetAwayUntil(accountID int64, until string) error {
	res, err := s.db.Exec("UPDATE line SET away_until = ? WHERE account_id = ?", until, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Available says whether a line's owner takes calls now.
func (s *Store) Available(lineName string) bool {
	var until string
	err := s.db.QueryRow("SELECT away_until FROM line WHERE name = ?", lineName).Scan(&until)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return true
	}
	return IsAvailable(until)
}

// IsAvailable reads a line's away_until.
func IsAvailable(awayUntil string) bool {
	return awayUntil == "" || awayUntil <= utc(time.Now())
}
