package store

import (
	"database/sql"
	"errors"
)

// Device is a browser (usually the owner's phone app) that gets push
// notifications. Endpoint and keys come from the browser's PushSubscription.
type Device struct {
	ID        int64
	Endpoint  string
	P256dh    string
	Auth      string
	UserAgent string
}

// SaveDevice adds or refreshes a push subscription for an account.
func (s *Store) SaveDevice(accountID int64, d Device) error {
	_, err := s.db.Exec(`INSERT INTO device (account_id, endpoint, p256dh, auth, user_agent) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET account_id = excluded.account_id, p256dh = excluded.p256dh,
			auth = excluded.auth, user_agent = excluded.user_agent`,
		accountID, d.Endpoint, d.P256dh, d.Auth, d.UserAgent)
	return err
}

// DevicesForLine lists the devices of the account that owns a line.
func (s *Store) DevicesForLine(line string) ([]Device, error) {
	rows, err := s.db.Query(`SELECT d.id, d.endpoint, d.p256dh, d.auth, d.user_agent
		FROM device d JOIN line l ON l.account_id = d.account_id WHERE l.name = ?`, line)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Endpoint, &d.P256dh, &d.Auth, &d.UserAgent); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// DeviceCount is how many devices of an account get notifications.
func (s *Store) DeviceCount(accountID int64) int {
	var n int
	s.db.QueryRow("SELECT count(*) FROM device WHERE account_id = ?", accountID).Scan(&n)
	return n
}

// DeleteDevice forgets a subscription (the push service said it's gone, or
// the owner turned notifications off).
func (s *Store) DeleteDevice(endpoint string) error {
	_, err := s.db.Exec("DELETE FROM device WHERE endpoint = ?", endpoint)
	return err
}

// Setting reads a server setting; "" when unset.
func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM setting WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO setting (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value", key, value)
	return err
}
