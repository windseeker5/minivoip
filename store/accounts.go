package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken = errors.New("an account with this email already exists")
	ErrBadPromo   = errors.New("this promo code isn't valid")
	ErrBadLogin   = errors.New("wrong email or password")
)

// MaxPassword is the longest password: bcrypt reads 72 bytes at most.
const MaxPassword = 72

// SessionLength is how long a login lasts.
const SessionLength = 30 * 24 * time.Hour

type User struct {
	ID        int64
	AccountID int64
	Email     string
	IsAdmin   bool // can manage promo codes
}

// CreateAccount makes an account with its first user. A promo code is
// optional here (web.signup requires one); when given it must be valid, it is
// used up by one, and the account gets its plan. Without one, the account is
// Basic. The code is checked first, so only someone holding a good code can
// learn that an email is taken.
func (s *Store) CreateAccount(email, password, promo string) (User, error) {
	email = strings.TrimSpace(email)
	promo = strings.TrimSpace(promo)
	// A wrong code costs nothing, and the slow hash happens outside the
	// transaction, which would hold up every other write.
	if promo != "" {
		var usable int
		err := s.db.QueryRow(`SELECT count(*) FROM promo_code WHERE code = ? AND revoked = 0
			AND (valid_from IS NULL OR valid_from <= datetime('now'))
			AND (valid_to IS NULL OR valid_to >= datetime('now'))
			AND (max_uses = 0 OR times_used < max_uses)`, promo).Scan(&usable)
		if err != nil {
			return User{}, err
		}
		if usable == 0 {
			return User{}, ErrBadPromo
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

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

	var taken int
	if err := tx.QueryRow("SELECT count(*) FROM user WHERE email = ?", email).Scan(&taken); err != nil {
		return User{}, err
	}
	if taken > 0 {
		return User{}, ErrEmailTaken
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
		// Takes as long as a wrong password, so timing doesn't tell which
		// emails have an account.
		passwordMatches(noPassword(), password)
		return User{}, ErrBadLogin
	}
	if err != nil {
		return User{}, err
	}
	if !passwordMatches([]byte(hash), password) {
		return User{}, ErrBadLogin
	}
	return u, nil
}

// Language is what an account's call notes are written in. It doesn't translate
// the site, which stays in English.
type Language string

const (
	French  Language = "fr"
	English Language = "en"
)

func (l Language) Valid() bool { return l == French || l == English }

// Language says what the account's notes are written in.
func (s *Store) Language(accountID int64) (Language, error) {
	var l Language
	err := s.db.QueryRow("SELECT language FROM account WHERE id = ?", accountID).Scan(&l)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return l, err
}

func (s *Store) SetLanguage(accountID int64, l Language) error {
	if !l.Valid() {
		return errors.New("unknown language " + string(l))
	}
	res, err := s.db.Exec("UPDATE account SET language = ? WHERE id = ?", l, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LoginSecret is what identifies an account and its current password, for
// signing things tied to them; "" for an unknown email.
func (s *Store) LoginSecret(email string) string {
	var id int64
	var hash string
	if err := s.db.QueryRow("SELECT id, password_hash FROM user WHERE email = ?", strings.TrimSpace(email)).Scan(&id, &hash); err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%s", id, hash)
}

// bcryptSlots caps how many password checks run at once: each takes all of a
// CPU core for a moment, and a burst must not stop the site.
var bcryptSlots = make(chan struct{}, max(2, runtime.NumCPU()/2))

func hashPassword(password string) ([]byte, error) {
	bcryptSlots <- struct{}{}
	defer func() { <-bcryptSlots }()
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

func passwordMatches(hash []byte, password string) bool {
	bcryptSlots <- struct{}{}
	defer func() { <-bcryptSlots }()
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

// noPassword is a hash no password matches, for accounts that don't exist.
var noPassword = sync.OnceValue(func() []byte {
	b := make([]byte, 32)
	rand.Read(b)
	hash, _ := bcrypt.GenerateFromPassword(b[:32], bcrypt.DefaultCost)
	return hash
})

// ChangePassword sets a new password after checking the current one, logs out
// every other device (keep is the session token that stays logged in) and
// stops notifications to all of them: a stolen login can't keep listening. A
// phone gets them again when its app opens.
func (s *Store) ChangePassword(userID int64, current, next, keep string) error {
	var hash string
	if err := s.db.QueryRow("SELECT password_hash FROM user WHERE id = ?", userID).Scan(&hash); err != nil {
		return err
	}
	if !passwordMatches([]byte(hash), current) {
		return ErrBadLogin
	}
	fresh, err := hashPassword(next)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE user SET password_hash = ? WHERE id = ?", string(fresh), userID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM session WHERE user_id = ? AND token <> ?", userID, tokenHash(keep)); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM device WHERE account_id = (SELECT account_id FROM user WHERE id = ?)", userID); err != nil {
		return err
	}
	return tx.Commit()
}

// tokenHash is what the database keeps of a session token: someone who reads
// a copy of the database can't log in with it.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// hashSessions converts the sessions from before migration 12.
func (s *Store) hashSessions() error {
	rows, err := s.db.Query("SELECT token FROM session WHERE hashed = 0")
	if err != nil {
		return err
	}
	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		tokens = append(tokens, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range tokens {
		if _, err := s.db.Exec("UPDATE session SET token = ?, hashed = 1 WHERE token = ?", tokenHash(t), t); err != nil {
			return err
		}
	}
	return nil
}

// NewSession starts a login and returns its token for the cookie.
func (s *Store) NewSession(userID int64) (string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(SessionLength).UTC()
	_, err := s.db.Exec("INSERT INTO session (token, hashed, user_id, expires_at) VALUES (?, 1, ?, ?)",
		tokenHash(token), userID, expires.Format(time.DateTime))
	return token, expires, err
}

// UserBySession returns the logged-in user, or ErrNotFound.
func (s *Store) UserBySession(token string) (User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT u.id, u.account_id, u.email, u.is_admin FROM session s JOIN user u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > datetime('now')`, tokenHash(token)).Scan(&u.ID, &u.AccountID, &u.Email, &u.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec("DELETE FROM session WHERE token = ?", tokenHash(token))
	return err
}
