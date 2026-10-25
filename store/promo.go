package store

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

var ErrPromoExists = errors.New("this promo code already exists")

// Plan is what an account gets: Basic (calls, the voice menu, voicemail)
// or Premium (adds video, screen sharing, the whiteboard and the meeting
// room). docs/vision.md "Two plans".
type Plan string

const (
	Basic   Plan = "basic"
	Premium Plan = "premium"
)

func (p Plan) Valid() bool { return p == Basic || p == Premium }

// Name is the plan as people read it: "Basic" or "Premium".
func (p Plan) Name() string {
	if p == Premium {
		return "Premium"
	}
	return "Basic"
}

type PromoCode struct {
	Code      string
	Plan      Plan   // the plan accounts signing up with it get
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

// CreatePromoCode adds a code that gives plan. Dates are "YYYY-MM-DD" or ""
// (the code works from the start of validFrom to the end of validTo, UTC).
func (s *Store) CreatePromoCode(code string, plan Plan, maxUses int, validFrom, validTo string) error {
	if !plan.Valid() {
		return errors.New("unknown plan " + string(plan))
	}
	var from, to any
	if validFrom != "" {
		from = validFrom + " 00:00:00"
	}
	if validTo != "" {
		to = validTo + " 23:59:59"
	}
	_, err := s.db.Exec("INSERT INTO promo_code (code, plan, max_uses, valid_from, valid_to) VALUES (?, ?, ?, ?, ?)",
		strings.ToUpper(code), plan, maxUses, from, to)
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
	rows, err := s.db.Query(`SELECT code, plan, coalesce(valid_from, ''), coalesce(valid_to, ''), max_uses, times_used, revoked, created_at
		FROM promo_code ORDER BY created_at DESC, code`)
	if err != nil {
		return nil, err
	}
	var list []PromoCode
	for rows.Next() {
		var p PromoCode
		if err := rows.Scan(&p.Code, &p.Plan, &p.ValidFrom, &p.ValidTo, &p.MaxUses, &p.TimesUsed, &p.Revoked, &p.CreatedAt); err != nil {
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

// Billing is an account's plan and how it pays.
type Billing struct {
	Plan    Plan
	Code    string // the promo code it signed up with, or ""
	Revoked bool   // that code was revoked: no longer free forever
}

func (s *Store) Billing(accountID int64) (Billing, error) {
	var b Billing
	var c sql.NullString
	err := s.db.QueryRow(`SELECT a.plan, a.promo_code, coalesce(p.revoked, 0) FROM account a
		LEFT JOIN promo_code p ON p.code = a.promo_code WHERE a.id = ?`, accountID).Scan(&b.Plan, &c, &b.Revoked)
	b.Code = c.String
	return b, err
}

// SetPlan moves an account to another plan.
func (s *Store) SetPlan(accountID int64, plan Plan) error {
	if !plan.Valid() {
		return errors.New("unknown plan " + string(plan))
	}
	res, err := s.db.Exec("UPDATE account SET plan = ? WHERE id = ?", plan, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Account is one row of the admin's account list.
type Account struct {
	ID     int64
	Emails []string
	Line   string // its line's name, or ""
	Billing
	CreatedAt string
}

// Accounts lists every account, newest first.
func (s *Store) Accounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT a.id, a.plan, coalesce(a.promo_code, ''), coalesce(p.revoked, 0), a.created_at,
		coalesce((SELECT name FROM line WHERE account_id = a.id), ''),
		coalesce((SELECT group_concat(email, ' ') FROM user WHERE account_id = a.id), '')
		FROM account a LEFT JOIN promo_code p ON p.code = a.promo_code ORDER BY a.created_at DESC, a.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Account
	for rows.Next() {
		var a Account
		var emails string
		if err := rows.Scan(&a.ID, &a.Plan, &a.Code, &a.Revoked, &a.CreatedAt, &a.Line, &emails); err != nil {
			return nil, err
		}
		a.Emails = strings.Fields(emails)
		list = append(list, a)
	}
	return list, rows.Err()
}

// SetAdmin marks whether a user can manage promo codes.
func (s *Store) SetAdmin(userID int64, admin bool) error {
	_, err := s.db.Exec("UPDATE user SET is_admin = ? WHERE id = ?", admin, userID)
	return err
}
