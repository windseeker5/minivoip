package store

import (
	"errors"
	"path/filepath"
	"testing"

	"minivoip/lines"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAccountsAndSessions(t *testing.T) {
	s := open(t)
	u, err := s.CreateAccount("ken@example.com", "secret-pass", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccount("KEN@example.com", "other-pass", ""); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("same email again: %v", err)
	}
	if _, err := s.Login("ken@example.com", "wrong"); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("wrong password: %v", err)
	}
	got, err := s.Login("ken@example.com", "secret-pass")
	if err != nil || got.ID != u.ID {
		t.Fatalf("login: %+v %v", got, err)
	}
	token, _, err := s.NewSession(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.UserBySession(token); err != nil || got.Email != "ken@example.com" {
		t.Fatalf("session: %+v %v", got, err)
	}
	s.DeleteSession(token)
	if _, err := s.UserBySession(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after logout: %v", err)
	}
}

func TestPromoCodes(t *testing.T) {
	s := open(t)
	s.db.Exec("INSERT INTO promo_code (code, max_uses) VALUES ('FAMILY', 1)")
	s.db.Exec("INSERT INTO promo_code (code, revoked) VALUES ('OLD', 1)")

	if _, err := s.CreateAccount("a@example.com", "password1", "nope"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("unknown code: %v", err)
	}
	if _, err := s.CreateAccount("b@example.com", "password1", "OLD"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("revoked code: %v", err)
	}
	if _, err := s.CreateAccount("c@example.com", "password1", "family"); err != nil {
		t.Fatalf("valid code: %v", err)
	}
	if _, err := s.CreateAccount("d@example.com", "password1", "FAMILY"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("used-up code: %v", err)
	}
}

func TestLineAndMenu(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	if err := s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B", "Joe"); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateAccount("sam@example.com", "password1", "")
	if err := s.CreateLine(other.AccountID, "joesbnb", "Sam's", "Sam"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("taken name: %v", err)
	}

	l, err := s.LineByName("joesbnb")
	if err != nil || len(l.Options) != 1 || l.Options[0].Action != lines.Ring {
		t.Fatalf("starter line: %+v %v", l, err)
	}
	if l.Menu() != "Press 1 to talk to Joe." {
		t.Fatalf("menu = %q", l.Menu())
	}

	opts := []lines.Option{
		{Key: "1", Label: "Bookings", Action: lines.Link, URL: "https://example.com/book"},
		{Key: "2", Label: "Talk to Joe", Action: lines.Ring},
	}
	if err := s.SaveMenu(u.AccountID, "Joe's B&B", "Joe", "Welcome!", opts); err != nil {
		t.Fatal(err)
	}
	l, _ = s.LineForAccount(u.AccountID)
	if l.Greeting != "Welcome!" || len(l.Options) != 2 || l.Options[0].URL != "https://example.com/book" {
		t.Fatalf("saved line: %+v", l)
	}
	if !s.LineExists("joesbnb") || s.LineExists("nobody") {
		t.Fatal("LineExists")
	}
}
