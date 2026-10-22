package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if err := s.SaveMenu(u.AccountID, "Joe's B&B", "Joe", "Welcome!", "", opts); err != nil {
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

func TestVoicemail(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B", "Joe")
	other, _ := s.CreateAccount("sam@example.com", "password1", "")

	v, err := s.AddVoicemail("joesbnb", "Talk to Joe", "audio/webm", 4200, strings.NewReader("fake opus"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddVoicemail("joesbnb", "", "audio/webm", 0, strings.NewReader("")); err == nil {
		t.Fatal("an empty recording should be refused")
	}
	if s.NewVoicemails(u.AccountID) != 1 {
		t.Fatal("one new message")
	}
	if _, _, err := s.VoicemailFile(other.AccountID, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("another account can't read it")
	}
	path, ct, err := s.VoicemailFile(u.AccountID, v.ID)
	if b, _ := os.ReadFile(path); err != nil || ct != "audio/webm" || string(b) != "fake opus" {
		t.Fatalf("file: %s %s %v", path, ct, err)
	}
	s.MarkListened(u.AccountID, v.ID)
	if list, _ := s.Voicemails(u.AccountID); len(list) != 1 || !list[0].Listened || list[0].Seconds() != 5 {
		t.Fatalf("list: %+v", list)
	}
	if err := s.DeleteVoicemail(u.AccountID, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the audio file should be deleted")
	}
}

func TestPromoAdmin(t *testing.T) {
	s := open(t)
	if err := CheckPromoCode("family"); err == nil {
		t.Fatal("codes are capitals")
	}
	if err := s.CreatePromoCode("FAMILY", 2, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePromoCode("FAMILY", 0, "", ""); !errors.Is(err, ErrPromoExists) {
		t.Fatalf("duplicate: %v", err)
	}
	s.CreatePromoCode("LATER", 0, "2999-01-01", "")
	if _, err := s.CreateAccount("b@example.com", "password1", "LATER"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("a code that isn't valid yet: %v", err)
	}
	u, err := s.CreateAccount("a@example.com", "password1", "family")
	if err != nil {
		t.Fatal(err)
	}
	if code, revoked, _ := s.Plan(u.AccountID); code != "FAMILY" || revoked {
		t.Fatalf("plan: %s %v", code, revoked)
	}
	list, _ := s.PromoCodes()
	var fam PromoCode
	for _, p := range list {
		if p.Code == "FAMILY" {
			fam = p
		}
	}
	if fam.TimesUsed != 1 || len(fam.Accounts) != 1 || fam.Accounts[0] != "a@example.com" || !fam.Usable(time.Now()) {
		t.Fatalf("FAMILY: %+v", fam)
	}
	if err := s.SetPromoRevoked("FAMILY", true); err != nil {
		t.Fatal(err)
	}
	if _, revoked, _ := s.Plan(u.AccountID); !revoked {
		t.Fatal("revoking shows on the account's plan")
	}
	if _, err := s.CreateAccount("c@example.com", "password1", "FAMILY"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("revoked code: %v", err)
	}
	if err := s.SetPromoRevoked("NOPE", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
}
