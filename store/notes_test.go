package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func notesAccount(t *testing.T, s *Store, email, line string) User {
	t.Helper()
	u, err := s.CreateAccount(email, "password1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateLine(u.AccountID, line, line); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlan(u.AccountID, Premium); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestNotesArePremiumOnly(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	s.SetPlan(u.AccountID, Basic)
	if _, err := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10); !errors.Is(err, ErrNotPremium) {
		t.Fatalf("Basic: %v", err)
	}
	s.SetPlan(u.AccountID, Premium)
	if _, err := s.AddNote("nope", NoteCall, "notes/a.webm", "audio/webm", 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such line: %v", err)
	}
	if _, err := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10); err != nil {
		t.Fatal(err)
	}
}

func TestNoteAudioStaysInTheDataFolder(t *testing.T) {
	s := open(t)
	notesAccount(t, s, "joe@example.com", "joes")
	for _, bad := range []string{"../secret", "/etc/passwd", "notes/../../secret"} {
		if _, err := s.AddNote("joes", NoteCall, bad, "audio/webm", 10); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNoteTakesTheAccountLanguage(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	first, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	s.SetLanguage(u.AccountID, English)
	second, _ := s.AddNote("joes", NoteCall, "notes/b.webm", "audio/webm", 10)
	if n, _ := s.NoteByID(u.AccountID, first); n.Language != French {
		t.Errorf("first: %q", n.Language)
	}
	if n, _ := s.NoteByID(u.AccountID, second); n.Language != English {
		t.Errorf("second: %q", n.Language)
	}
}

func TestClaimTakesOldestOneAtATime(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	first, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	second, _ := s.AddNote("joes", NoteCall, "notes/b.webm", "audio/webm", 10)
	n, ok, err := s.ClaimNote()
	if err != nil || !ok || n.ID != first || n.Status != NoteWorking || n.Tries != 1 {
		t.Fatalf("%+v %v %v", n, ok, err)
	}
	if n, _, _ := s.ClaimNote(); n.ID != second {
		t.Fatalf("second claim: %d", n.ID)
	}
	if _, ok, _ := s.ClaimNote(); ok {
		t.Fatal("nothing left to claim")
	}
	// A note waiting for its next try isn't taken before its time.
	s.RetryNote(first, 600)
	if _, ok, _ := s.ClaimNote(); ok {
		t.Fatal("taken before its time")
	}
	if n, _ := s.NoteByID(u.AccountID, first); n.Status != NoteWaiting {
		t.Fatalf("status: %s", n.Status)
	}
	s.db.Exec("UPDATE note SET next_try = datetime('now') WHERE id = ?", first)
	if n, ok, _ := s.ClaimNote(); !ok || n.Tries != 2 {
		t.Fatalf("%+v %v", n, ok)
	}
	if err := s.ReleaseNotes(); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.NoteByID(u.AccountID, first); n.Status != NoteWaiting || n.Tries != 2 {
		t.Fatalf("released: %s, %d tries", n.Status, n.Tries)
	}
}

func TestNotesBelongToTheirAccount(t *testing.T) {
	s := open(t)
	joe := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	id, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	if _, err := s.NoteByID(sue.AccountID, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else reads it: %v", err)
	}
	if err := s.DeleteNote(sue.AccountID, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else deletes it: %v", err)
	}
	if list, _ := s.Notes(sue.AccountID); len(list) != 0 {
		t.Fatalf("someone else lists it: %+v", list)
	}
	if list, _ := s.Notes(joe.AccountID); len(list) != 1 || list[0].ID != id {
		t.Fatalf("the owner's list: %+v", list)
	}
}

func TestNotesListNewestFirst(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	first, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	second, _ := s.AddNote("joes", NoteRoom, "notes/b.webm", "audio/webm", 10)
	list, err := s.Notes(u.AccountID)
	if err != nil || len(list) != 2 || list[0].ID != second || list[1].ID != first {
		t.Fatalf("%+v %v", list, err)
	}
}

// A long answer is cut to what a page can hold.
func TestFinishNoteCuts(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	id, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	s.ClaimNote()
	actions := make([]string, maxNoteActions+5)
	for i := range actions {
		actions[i] = strings.Repeat("é", maxNoteAction+5)
	}
	if err := s.FinishNote(id, strings.Repeat("é", maxNoteSummary+5), actions); err != nil {
		t.Fatal(err)
	}
	n, _ := s.NoteByID(u.AccountID, id)
	if len([]rune(n.Summary)) != maxNoteSummary || len(n.NextActions) != maxNoteActions || len([]rune(n.NextActions[0])) != maxNoteAction {
		t.Fatalf("%d %d %d", len([]rune(n.Summary)), len(n.NextActions), len([]rune(n.NextActions[0])))
	}
}

func TestNoteHours(t *testing.T) {
	s := open(t)
	joe := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	s.ChargeNotes(joe.AccountID, 100)
	s.ChargeNotes(joe.AccountID, 50)
	s.ChargeNotes(sue.AccountID, 7)
	// Last month doesn't count this month, or today.
	s.db.Exec("INSERT INTO note_usage (account_id, day, seconds) VALUES (?, date('now', 'start of month', '-1 day'), 9999)", joe.AccountID)
	if n, _ := s.NoteSecondsThisMonth(joe.AccountID); n != 150 {
		t.Errorf("Joe's month: %d", n)
	}
	if n, _ := s.NoteSecondsThisMonth(sue.AccountID); n != 7 {
		t.Errorf("Sue's month: %d", n)
	}
	if n, _ := s.NoteSecondsToday(); n != 157 {
		t.Errorf("today: %d", n)
	}
}

func TestNotesCleanedUpAfterAYear(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	os.MkdirAll(filepath.Join(s.Dir, "notes"), 0o700)
	for _, name := range []string{"old-call.webm", "old-mail.webm", "new.webm"} {
		os.WriteFile(filepath.Join(s.Dir, "notes", name), []byte("x"), 0o600)
	}
	oldCall, _ := s.AddNote("joes", NoteCall, "notes/old-call.webm", "audio/webm", 10)
	oldMail, _ := s.AddNote("joes", NoteVoicemail, "notes/old-mail.webm", "audio/webm", 10)
	recent, _ := s.AddNote("joes", NoteCall, "notes/new.webm", "audio/webm", 10)
	s.db.Exec("UPDATE note SET created_at = datetime('now', '-366 days') WHERE id IN (?, ?)", oldCall, oldMail)
	s.ChargeNotes(u.AccountID, 5)
	s.db.Exec("INSERT INTO note_usage (account_id, day, seconds) VALUES (?, date('now', '-100 days'), 9)", u.AccountID)
	if err := s.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NoteByID(u.AccountID, oldCall); !errors.Is(err, ErrNotFound) {
		t.Errorf("old call: %v", err)
	}
	if _, err := s.NoteByID(u.AccountID, oldMail); !errors.Is(err, ErrNotFound) {
		t.Errorf("old voicemail: %v", err)
	}
	if _, err := s.NoteByID(u.AccountID, recent); err != nil {
		t.Errorf("recent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "notes", "old-call.webm")); err == nil {
		t.Error("an old call's recording stayed")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "notes", "old-mail.webm")); err != nil {
		t.Error("a voicemail's audio has its own life")
	}
	var rows int
	s.db.QueryRow("SELECT count(*) FROM note_usage").Scan(&rows)
	if rows != 1 {
		t.Errorf("hours kept: %d rows", rows)
	}
}

// Deleting an account takes its notes and hours with it.
func TestNotesGoWithTheAccount(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	s.ChargeNotes(u.AccountID, 5)
	if _, err := s.db.Exec("DELETE FROM account WHERE id = ?", u.AccountID); err != nil {
		t.Fatal(err)
	}
	var notes, usage int
	s.db.QueryRow("SELECT count(*) FROM note").Scan(&notes)
	s.db.QueryRow("SELECT count(*) FROM note_usage").Scan(&usage)
	if notes != 0 || usage != 0 {
		t.Fatalf("%d notes, %d usage rows", notes, usage)
	}
}
