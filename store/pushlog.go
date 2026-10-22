package store

// The push log is a delivery receipt for every push notification: the
// server writes a row when it sends one, and the phone's service worker
// reports back when it showed it. It answers "did the phone ring, and how
// late?" Rows older than 14 days are dropped.

// LogPush records a notification about to be sent to a device and returns
// its id (sent in the notification so the phone can report back).
func (s *Store) LogPush(deviceID int64, line, callID, kind string) (int64, error) {
	s.db.Exec("DELETE FROM push_log WHERE sent_at < strftime('%Y-%m-%d %H:%M:%f', 'now', '-14 days')")
	res, err := s.db.Exec("INSERT INTO push_log (device_id, line, call_id, kind) VALUES (?, ?, ?, ?)", deviceID, line, callID, kind)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// PushResult stores what the push service answered: "ok" or the error.
func (s *Store) PushResult(id int64, result string) error {
	_, err := s.db.Exec("UPDATE push_log SET result = ? WHERE id = ?", result, id)
	return err
}

// PushShown stores the phone's receipt. It only matches a notification sent
// to one of the account's devices, and reports whether it did.
func (s *Store) PushShown(accountID, id int64, shownAs, note string) (bool, error) {
	res, err := s.db.Exec(`UPDATE push_log SET shown_at = strftime('%Y-%m-%d %H:%M:%f', 'now'), shown_as = ?, phone_note = ?
		WHERE id = ? AND shown_at IS NULL AND device_id IN (SELECT id FROM device WHERE account_id = ?)`,
		shownAs, note, id, accountID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// PushLogEntry is one row of the push log.
type PushLogEntry struct {
	ID       int64
	DeviceID int64
	Kind     string
	CallID   string
	Result   string
	SentAt   string
	ShownAt  string // "" when the phone never reported it
	ShownAs  string
}

// PushLog lists a line's latest notifications, newest first.
func (s *Store) PushLog(line string, limit int) ([]PushLogEntry, error) {
	rows, err := s.db.Query(`SELECT id, device_id, kind, call_id, result, sent_at, coalesce(shown_at, ''), shown_as
		FROM push_log WHERE line = ? ORDER BY id DESC LIMIT ?`, line, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PushLogEntry
	for rows.Next() {
		var e PushLogEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Kind, &e.CallID, &e.Result, &e.SentAt, &e.ShownAt, &e.ShownAs); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}
