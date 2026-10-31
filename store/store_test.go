package store

import (
	"bytes"
	"errors"
	"image"
	"image/png"
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

func TestPlans(t *testing.T) {
	s := open(t)
	u, err := s.CreateAccount("basic@example.com", "password1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLine(u.AccountID, "shop", "The Shop"); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Billing(u.AccountID); b.Plan != Basic {
		t.Fatalf("a new account without a code is Basic, got %q", b.Plan)
	}
	if l, _ := s.LineByName("shop"); l.Premium || s.HasRoom("shop") {
		t.Fatal("a Basic line has no video and no meeting room")
	}
	if err := s.SetPlan(u.AccountID, Premium); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.LineByName("shop"); !l.Premium || !s.HasRoom("shop") {
		t.Fatal("a Premium line has video and a meeting room")
	}
	if s.HasRoom("nope") {
		t.Fatal("no room for a line that doesn't exist")
	}
	if err := s.SetPlan(u.AccountID, "gold"); err == nil {
		t.Fatal("unknown plan accepted")
	}
	if err := s.SetPlan(999, Basic); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	list, err := s.Accounts()
	if err != nil || len(list) != 1 || list[0].Plan != Premium || list[0].Line != "shop" || list[0].Emails[0] != "basic@example.com" {
		t.Fatalf("accounts: %+v %v", list, err)
	}
}

// The landing page's "Get Premium" list keeps each email once.
func TestPremiumRequests(t *testing.T) {
	s := open(t)
	for _, e := range []string{"ann@example.com", "ANN@example.com", "bob@example.com"} {
		if err := s.AskPremium(e); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.PremiumRequests()
	if err != nil || len(list) != 2 {
		t.Fatalf("requests: %+v %v", list, err)
	}
}

// An admin's account becomes Premium when the plans migration runs.
func TestPlansMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := s.CreateAccount("admin@example.com", "password1", "")
	other, _ := s.CreateAccount("other@example.com", "password1", "")
	s.SetAdmin(admin.ID, true)
	s.db.Exec("UPDATE account SET plan = 'basic'")
	s.db.Exec("PRAGMA user_version = 7")
	s.db.Exec("ALTER TABLE account DROP COLUMN plan")
	s.db.Exec("ALTER TABLE promo_code DROP COLUMN plan")
	s.db.Exec("ALTER TABLE line DROP COLUMN logo")
	s.db.Exec("DROP TABLE premium_request")
	s.Close()
	if s, err = Open(path); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if b, _ := s.Billing(admin.AccountID); b.Plan != Premium {
		t.Errorf("admin's account: %q", b.Plan)
	}
	if b, _ := s.Billing(other.AccountID); b.Plan != Basic {
		t.Errorf("other account: %q", b.Plan)
	}
}

func TestLineAndMenu(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	if err := s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B"); err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateAccount("sam@example.com", "password1", "")
	if err := s.CreateLine(other.AccountID, "joesbnb", "Sam's"); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("taken name: %v", err)
	}

	l, err := s.LineByName("joesbnb")
	if err != nil || len(l.Options) != 1 || l.Options[0].Action != lines.Ring {
		t.Fatalf("starter line: %+v %v", l, err)
	}
	if l.Menu() != "Press 1 to talk to us." {
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
	s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B")
	other, _ := s.CreateAccount("sam@example.com", "password1", "")

	v, err := s.AddVoicemail("joesbnb", "Talk to Joe", "", "audio/webm", 4200, strings.NewReader("fake opus"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddVoicemail("joesbnb", "", "", "audio/webm", 0, strings.NewReader("")); err == nil {
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
	if err := s.CreatePromoCode("FAMILY", Premium, 2, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePromoCode("FAMILY", Basic, 0, "", ""); !errors.Is(err, ErrPromoExists) {
		t.Fatalf("duplicate: %v", err)
	}
	s.CreatePromoCode("LATER", Basic, 0, "2999-01-01", "")
	if _, err := s.CreateAccount("b@example.com", "password1", "LATER"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("a code that isn't valid yet: %v", err)
	}
	u, err := s.CreateAccount("a@example.com", "password1", "family")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Billing(u.AccountID); b.Code != "FAMILY" || b.Revoked || b.Plan != Premium {
		t.Fatalf("billing: %+v", b)
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
	if b, _ := s.Billing(u.AccountID); !b.Revoked {
		t.Fatal("revoking shows on the account's plan")
	}
	if _, err := s.CreateAccount("c@example.com", "password1", "FAMILY"); !errors.Is(err, ErrBadPromo) {
		t.Fatalf("revoked code: %v", err)
	}
	if err := s.SetPromoRevoked("NOPE", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
}

func TestPushLog(t *testing.T) {
	s := open(t)
	joe, _ := s.CreateAccount("joe@example.com", "password1", "")
	ann, _ := s.CreateAccount("ann@example.com", "password1", "")
	s.CreateLine(joe.AccountID, "joesbnb", "Joe's B&B")
	s.SaveDevice(joe.AccountID, Device{Endpoint: "https://push.example/joe", P256dh: "k", Auth: "a"})
	s.SaveDevice(ann.AccountID, Device{Endpoint: "https://push.example/ann", P256dh: "k", Auth: "a"})
	d, _ := s.DevicesForLine("joesbnb")

	id, err := s.LogPush(d[0].ID, "joesbnb", "abc", "ring")
	if err != nil {
		t.Fatal(err)
	}
	s.PushResult(id, "ok")
	if ok, _ := s.PushShown(ann.AccountID, id, "ring", ""); ok {
		t.Fatal("another account's phone can't sign a receipt")
	}
	if ok, err := s.PushShown(joe.AccountID, id, "ring", "app not on screen"); !ok || err != nil {
		t.Fatalf("receipt: %v %v", ok, err)
	}
	if ok, _ := s.PushShown(joe.AccountID, id, "late", ""); ok {
		t.Fatal("a receipt counts once")
	}
	log, _ := s.PushLog("joesbnb", 10)
	if len(log) != 1 || log[0].Result != "ok" || log[0].ShownAs != "ring" || log[0].ShownAt == "" {
		t.Fatalf("push log = %+v", log)
	}
}

func TestCallHistory(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B")
	other, _ := s.CreateAccount("sam@example.com", "password1", "")
	s.CreateLine(other.AccountID, "samsshop", "Sam's Shop")

	t0 := time.Date(2026, 1, 10, 14, 0, 0, 0, time.UTC)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.LogCall("joesbnb", CallRecord{CallID: "a1", Option: "Book a room", Outcome: "answered",
		Started: t0, Answered: t0.Add(5 * time.Second), Ended: t0.Add(65 * time.Second)}))
	must(s.LogCall("joesbnb", CallRecord{CallID: "m1", Outcome: "missed", Started: t0.Add(time.Hour), Ended: t0.Add(time.Hour + 30*time.Second)}))
	must(s.LogCall("samsshop", CallRecord{CallID: "x1", Outcome: "missed", Started: t0, Ended: t0}))
	// A voicemail after the missed call, and one from a voicemail choice.
	_, err := s.AddVoicemail("joesbnb", "", "m1", "audio/webm", 3000, strings.NewReader("a"))
	must(err)
	_, err = s.AddVoicemail("joesbnb", "Leave a message", "", "audio/webm", 3000, strings.NewReader("b"))
	must(err)

	list, err := s.CallHistory(u.AccountID, 50)
	must(err)
	if len(list) != 3 {
		t.Fatalf("history: %+v", list)
	}
	// The voicemail choice was saved just now, so it comes first.
	if list[0].Outcome != "" || list[0].Voicemail == nil || list[0].Option != "Leave a message" {
		t.Fatalf("voicemail choice: %+v", list[0])
	}
	if list[1].Outcome != "missed" || list[1].Voicemail == nil || list[1].Voicemail.CallID != "m1" {
		t.Fatalf("missed call with voicemail: %+v", list[1])
	}
	if list[2].Outcome != "answered" || list[2].TalkSeconds != 60 || list[2].Voicemail != nil || list[2].At != "2026-01-10 14:00:00" {
		t.Fatalf("answered call: %+v", list[2])
	}
}

func TestAvailable(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B")
	if !s.Available("joesbnb") {
		t.Fatal("a new line is available")
	}
	s.SetAwayUntil(u.AccountID, Away)
	if s.Available("joesbnb") {
		t.Fatal("Off means away")
	}
	s.SetAwayUntil(u.AccountID, time.Now().Add(-time.Minute).UTC().Format(time.DateTime))
	if !s.Available("joesbnb") {
		t.Fatal("a pause that ended means available again")
	}
	if l, _ := s.LineForAccount(u.AccountID); l.AwayUntil == "" {
		t.Fatal("the line reads away_until")
	}
}

func TestLogo(t *testing.T) {
	s := open(t)
	u, _ := s.CreateAccount("joe@example.com", "password1", "")
	if err := s.CreateLine(u.AccountID, "joesbnb", "Joe's B&B"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "hello, not a picture", "\x89PNG\r\n\x1a\nbroken"} {
		if err := s.SetLogo(u.AccountID, strings.NewReader(bad)); !errors.Is(err, ErrBadLogo) {
			t.Errorf("SetLogo(%q) = %v, want ErrBadLogo", bad, err)
		}
	}
	if err := s.SetLogo(u.AccountID, bytes.NewReader(tinyPNG(t))); err != nil {
		t.Fatal(err)
	}
	l, _ := s.LineByName("joesbnb")
	first := s.LogoPath(l.Logo)
	if first == "" || !strings.HasSuffix(l.Logo, ".png") {
		t.Fatalf("logo name %q", l.Logo)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	// A new logo replaces the old file; removing it leaves none.
	s.SetLogo(u.AccountID, bytes.NewReader(tinyPNG(t)))
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Errorf("old logo still there: %v", err)
	}
	l, _ = s.LineByName("joesbnb")
	second := s.LogoPath(l.Logo)
	if err := s.RemoveLogo(u.AccountID); err != nil {
		t.Fatal(err)
	}
	l, _ = s.LineByName("joesbnb")
	if _, err := os.Stat(second); l.Logo != "" || !os.IsNotExist(err) {
		t.Errorf("after remove: logo %q, file %v", l.Logo, err)
	}
	if s.LogoPath("../test.db") != "" {
		t.Error("LogoPath takes any name")
	}
}

func tinyPNG(t *testing.T) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
