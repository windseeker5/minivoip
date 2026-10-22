package store

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrPromoExists = errors.New("this promo code already exists")

type PromoCode struct {
	Code      string
	ValidFrom string // UTC "YYYY-MM-DD HH:MM:SS", or "" for no start
	ValidTo   string // or "" for no end
	MaxUses   int    // 0 = unlimited
	TimesUsed int
	Revoked   bool
	CreatedAt string
	Accounts  []string // sign-up emails of the accounts that used it
}

// Usable says whether a new sign up could use the code now.
func (p PromoCode) Usable(now time.Time) bool {
	t := now.UTC().Format(time.DateTime)
	return !p.Revoked && (p.ValidFrom == "" || p.ValidFrom <= t) && (p.ValidTo == "" || p.ValidTo >= t) &&
		(p.MaxUses == 0 || p.TimesUsed < p.MaxUses)
}

var promoRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{1,28}[A-Z0-9]$`)

// CheckPromoCode says why a new code isn't acceptable, or nil. Codes are
// stored in capitals; sign up accepts any case.
func CheckPromoCode(code string) error {
	if !promoRe.MatchString(code) {
		return errors.New("Use 3 to 30 letters, numbers or dashes, like FAMILY or SPRING-2027.")
	}
	return nil
}

// CreatePromoCode adds a code. Dates are "YYYY-MM-DD" or "" (the code works
// from the start of validFrom to the end of validTo, UTC).
func (s *Store) CreatePromoCode(code string, maxUses int, validFrom, validTo string) error {
	var from, to any
	if validFrom != "" {
		from = validFrom + " 00:00:00"
	}
	if validTo != "" {
		to = validTo + " 23:59:59"
	}
	_, err := s.db.Exec("INSERT INTO promo_code (code, max_uses, valid_from, valid_to) VALUES (?, ?, ?, ?)",
		strings.ToUpper(code), maxUses, from, to)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrPromoExists
	}
	return err
}

// SetPromoRevoked stops (or allows again) new sign ups with a code. Accounts
// that used a revoked code are no longer free forever.
func (s *Store) SetPromoRevoked(code string, revoked bool) error {
	res, err := s.db.Exec("UPDATE promo_code SET revoked = ? WHERE code = ?", revoked, code)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PromoCodes lists every code, newest first, with who used it.
func (s *Store) PromoCodes() ([]PromoCode, error) {
	rows, err := s.db.Query(`SELECT code, coalesce(valid_from, ''), coalesce(valid_to, ''), max_uses, times_used, revoked, created_at
		FROM promo_code ORDER BY created_at DESC, code`)
	if err != nil {
		return nil, err
	}
	var list []PromoCode
	for rows.Next() {
		var p PromoCode
		if err := rows.Scan(&p.Code, &p.ValidFrom, &p.ValidTo, &p.MaxUses, &p.TimesUsed, &p.Revoked, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range list {
		accts, err := s.db.Query("SELECT name FROM account WHERE promo_code = ? ORDER BY created_at", list[i].Code)
		if err != nil {
			return nil, err
		}
		for accts.Next() {
			var name string
			accts.Scan(&name)
			list[i].Accounts = append(list[i].Accounts, name)
		}
		accts.Close()
	}
	return list, nil
}

// Plan describes what an account pays: its promo code and whether the code
// still holds. code is "" without one.
func (s *Store) Plan(accountID int64) (code string, revoked bool, err error) {
	var c sql.NullString
	err = s.db.QueryRow(`SELECT a.promo_code, coalesce(p.revoked, 0) FROM account a
		LEFT JOIN promo_code p ON p.code = a.promo_code WHERE a.id = ?`, accountID).Scan(&c, &revoked)
	return c.String, revoked, err
}

// SetAdmin marks whether a user can manage promo codes.
func (s *Store) SetAdmin(userID int64, admin bool) error {
	_, err := s.db.Exec("UPDATE user SET is_admin = ? WHERE id = ?", admin, userID)
	return err
}
