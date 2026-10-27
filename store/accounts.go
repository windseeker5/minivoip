package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken = errors.New("an account with this email already exists")
	ErrBadPromo   = errors.New("this promo code isn't valid")
	ErrBadLogin   = errors.New("wrong email or password")
)

// SessionLength is how long a login lasts.
const SessionLength = 30 * 24 * time.Hour

type User struct {
	ID        int64
	AccountID int64
	Email     string
	IsAdmin   bool // can manage promo codes (see web.Server.Admins)
}

// CreateAccount makes an account with its first user. A promo code is
// optional; when given it must be valid, it is used up by one, and the
// account gets its plan. Without one, the account is Basic.
func (s *Store) CreateAccount(email, password, promo string) (User, error) {
	email = strings.TrimSpace(email)
	promo = strings.TrimSpace(promo)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var taken int
	if err := tx.QueryRow("SELECT count(*) FROM user WHERE email = ?", email).Scan(&taken); err != nil {
		return User{}, err
	}
	if taken > 0 {
		return User{}, ErrEmailTaken
	}

	var promoArg any
	plan := Basic
	if promo != "" {
		res, err := tx.Exec(`
			UPDATE promo_code SET times_used = times_used + 1
			WHERE code = ? AND revoked = 0
			  AND (valid_from IS NULL OR valid_from <= datetime('now'))
			  AND (valid_to IS NULL OR valid_to >= datetime('now'))
			  AND (max_uses = 0 OR times_used < max_uses)`, promo)
		if err != nil {
			return User{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return User{}, ErrBadPromo
		}
		if err := tx.QueryRow("SELECT code, plan FROM promo_code WHERE code = ?", promo).Scan(&promo, &plan); err != nil {
			return User{}, err
		}
		promoArg = promo
	}

	res, err := tx.Exec("INSERT INTO account (name, promo_code, plan) VALUES (?, ?, ?)", email, promoArg, plan)
	if err != nil {
		return User{}, err
	}
	accountID, _ := res.LastInsertId()
	res, err = tx.Exec("INSERT INTO user (account_id, email, password_hash) VALUES (?, ?, ?)", accountID, email, string(hash))
	if err != nil {
		return User{}, err
	}
	userID, _ := res.LastInsertId()
	return User{ID: userID, AccountID: accountID, Email: email}, tx.Commit()
}

// Login checks an email and password.
func (s *Store) Login(email, password string) (User, error) {
	var u User
	var hash string
	err := s.db.QueryRow("SELECT id, account_id, email, password_hash, is_admin FROM user WHERE email = ?", strings.TrimSpace(email)).
		Scan(&u.ID, &u.AccountID, &u.Email, &hash, &u.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrBadLogin
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrBadLogin
	}
	return u, nil
}

// NewSession starts a login and returns its token for the cookie.
func (s *Store) NewSession(userID int64) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(SessionLength).UTC()
	_, err := s.db.Exec("INSERT INTO session (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expires.Format(time.DateTime))
	return token, expires, err
}

// UserBySession returns the logged-in user, or ErrNotFound.
func (s *Store) UserBySession(token string) (User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT u.id, u.account_id, u.email, u.is_admin FROM session s JOIN user u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > datetime('now')`, token).Scan(&u.ID, &u.AccountID, &u.Email, &u.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec("DELETE FROM session WHERE token = ?", token)
	return err
}
