package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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
	for _, bad := range []string{"", "../secret", "/etc/passwd", "notes/../../secret"} {
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

func voicemail(t *testing.T, s *Store, line string, ms int64) Voicemail {
	t.Helper()
	v, err := s.AddVoicemail(line, "", "", "audio/webm", ms, strings.NewReader("sound"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVoicemailNote(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	if ok, err := s.NoteVoicemail(voicemail(t, s, "joes", MinNoteMS-1)); ok || err != nil {
		t.Fatalf("a very short message: %v %v", ok, err)
	}
	v := voicemail(t, s, "joes", MinNoteMS)
	if ok, err := s.NoteVoicemail(v); !ok || err != nil {
		t.Fatalf("a message: %v %v", ok, err)
	}
	list, _ := s.Notes(u.AccountID)
	if len(list) != 1 || list[0].Kind != NoteVoicemail || list[0].AudioType != "audio/webm" || list[0].Seconds != 2 {
		t.Fatalf("notes: %+v", list)
	}
	if b, err := os.ReadFile(s.NoteAudioPath(list[0])); err != nil || string(b) != "sound" {
		t.Fatalf("its recording: %q %v", b, err)
	}
	s.SetPlan(u.AccountID, Basic)
	if ok, err := s.NoteVoicemail(voicemail(t, s, "joes", 9000)); ok || err != nil {
		t.Fatalf("Basic: %v %v", ok, err)
	}
	if ok, err := s.NoteVoicemail(Voicemail{ID: 9999, DurationMS: 9000}); ok || !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such message: %v %v", ok, err)
	}
}

// A message that expires leaves its note; one the owner deletes takes it along,
// and only its own.
func TestVoicemailNoteLife(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	old := voicemail(t, s, "joes", 9000)
	s.NoteVoicemail(old)
	gone := voicemail(t, s, "joes", 9000)
	s.NoteVoicemail(gone)
	s.db.Exec("UPDATE voicemail SET created_at = datetime('now', ?) WHERE id = ?", fmt.Sprintf("-%d days", VoicemailDays+1), old.ID)
	if err := s.Cleanup(); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Notes(u.AccountID)
	if len(list) != 2 {
		t.Fatalf("a message that expired took its note: %+v", list)
	}
	if err := s.DeleteVoicemail(sue.AccountID, gone.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else deletes it: %v", err)
	}
	if err := s.DeleteVoicemail(u.AccountID, gone.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Notes(u.AccountID); len(list) != 1 {
		t.Fatalf("after delete: %+v", list)
	}
}

// Past a line's share of a day, no note waits for the services, and other lines
// aren't touched.
func TestVoicemailNotesPerDay(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	for range MaxVoicemailNotesPerDay {
		s.NoteVoicemail(voicemail(t, s, "joes", 9000))
	}
	if ok, err := s.NoteVoicemail(voicemail(t, s, "joes", 9000)); !ok || err != nil {
		t.Fatalf("one too many: %v %v", ok, err)
	}
	list, _ := s.Notes(u.AccountID)
	if len(list) != MaxVoicemailNotesPerDay+1 {
		t.Fatalf("%d notes", len(list))
	}
	if n := list[0]; n.Status != NoteSkipped || n.Reason != NoteDayLimit || n.Audio != "" {
		t.Fatalf("one too many: %+v", n)
	}
	s.NoteVoicemail(voicemail(t, s, "sues", 9000))
	if theirs, _ := s.Notes(sue.AccountID); len(theirs) != 1 || theirs[0].Status != NoteWaiting {
		t.Fatalf("another line: %+v", theirs)
	}
}

func TestTakeNotesSwitch(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	if on, err := s.TakeNotes(u.AccountID); !on || err != nil {
		t.Fatalf("on by default: %v %v", on, err)
	}
	if err := s.SetTakeNotes(u.AccountID, false); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.TakeNotes(u.AccountID); on {
		t.Fatal("still on")
	}
	if on, _ := s.TakeNotes(sue.AccountID); !on {
		t.Fatal("someone else's switch changed")
	}
	if ok, err := s.NoteVoicemail(voicemail(t, s, "joes", 9000)); ok || err != nil {
		t.Fatalf("a voicemail with the switch off: %v %v", ok, err)
	}
	if _, err := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10); !errors.Is(err, ErrNotesOff) {
		t.Fatalf("a call with the switch off: %v", err)
	}
	if list, _ := s.Notes(u.AccountID); len(list) != 0 {
		t.Fatalf("notes: %+v", list)
	}
	if err := s.SetTakeNotes(999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	if _, err := s.TakeNotes(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
}

func TestVoicemailsKnowTheirNote(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	v := voicemail(t, s, "joes", 9000)
	short := voicemail(t, s, "joes", 500)
	s.NoteVoicemail(v)
	s.NoteVoicemail(short)
	notes, _ := s.Notes(u.AccountID)
	list, _ := s.Voicemails(u.AccountID)
	if len(list) != 2 || list[1].NoteID != notes[0].ID || list[0].NoteID != 0 {
		t.Fatalf("messages: %+v", list)
	}
	if notes[0].VoicemailID != v.ID {
		t.Fatalf("the note's message: %d", notes[0].VoicemailID)
	}
}

// The list leaves the words out; the page has them.
func TestNotesListHasNoTranscripts(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	id, _ := s.AddNote("joes", NoteCall, "notes/a.webm", "audio/webm", 10)
	n, _, _ := s.ClaimNote()
	if err := s.SaveTranscript(n, []NoteLine{{Speaker: "speaker_0", Text: "Bonjour"}}, 10); err != nil {
		t.Fatal(err)
	}
	s.FinishNote(id, "Joe", []string{"Call"})
	list, err := s.Notes(u.AccountID)
	if err != nil || len(list) != 1 || list[0].Transcript != nil || list[0].Summary != "Joe" || list[0].NextActions[0] != "Call" {
		t.Fatalf("%+v %v", list, err)
	}
	if one, _ := s.NoteByID(u.AccountID, id); len(one.Transcript) != 1 {
		t.Fatalf("the page: %+v", one)
	}
}

// A line knows if it takes notes, so its pages can say so.
func TestLineKnowsAboutNotes(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	l, err := s.LineByName("joes")
	if err != nil || !l.Notes || l.Language != "fr" {
		t.Fatalf("Premium: %+v %v", l, err)
	}
	s.SetLanguage(u.AccountID, English)
	if l, _ = s.LineForAccount(u.AccountID); l.Language != "en" {
		t.Fatalf("English: %q", l.Language)
	}
	s.SetTakeNotes(u.AccountID, false)
	if l, _ = s.LineByName("joes"); l.Notes {
		t.Fatal("the switch is off")
	}
	s.SetTakeNotes(u.AccountID, true)
	s.SetPlan(u.AccountID, Basic)
	if l, _ = s.LineByName("joes"); l.Notes {
		t.Fatal("Basic has no notes, whatever its switch says")
	}
}

func TestRecordingInPieces(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	n, err := s.StartRecording("joes", "c1", "audio/webm")
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != NoteCall || n.Status != NoteWaiting || !strings.HasPrefix(n.Audio, "notes/call-") || !strings.HasSuffix(n.Audio, ".webm") {
		t.Fatalf("note: %+v", n)
	}
	if again, err := s.StartRecording("joes", "c1", "audio/webm"); err != nil || again.ID != n.ID {
		t.Fatalf("a call has one note: %+v %v", again, err)
	}
	// Its turn doesn't come while pieces are arriving.
	if _, ok, _ := s.ClaimNote(); ok {
		t.Fatal("claimed before the last piece")
	}
	if err := s.AppendRecording(u.AccountID, "c1", 0, []byte("one ")); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendRecording(u.AccountID, "c1", 0, []byte("one ")); err != nil {
		t.Fatalf("a piece sent again: %v", err)
	}
	if err := s.AppendRecording(u.AccountID, "c1", 5, []byte("x")); !errors.Is(err, ErrRecordingStale) {
		t.Fatalf("a piece from the future: %v", err)
	}
	if err := s.AppendRecording(sue.AccountID, "c1", 1, []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else's: %v", err)
	}
	if err := s.AppendRecording(u.AccountID, "no-such-call", 0, []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no such call: %v", err)
	}
	if err := s.AppendRecording(u.AccountID, "c1", 1, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRecording(sue.AccountID, "c1"); !errors.Is(err, ErrRecordingStale) {
		t.Fatalf("someone else finishing: %v", err)
	}
	if err := s.FinishRecording(u.AccountID, "c1"); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimNote()
	if !ok || err != nil || claimed.ID != n.ID {
		t.Fatalf("its turn: %+v %v %v", claimed, ok, err)
	}
	if b, _ := os.ReadFile(s.NoteAudioPath(claimed)); string(b) != "one two" {
		t.Fatalf("the file: %q", b)
	}
	if err := s.AppendRecording(u.AccountID, "c1", 2, []byte("late")); !errors.Is(err, ErrRecordingStale) {
		t.Fatalf("a piece after its turn: %v", err)
	}
	if err := s.SaveTranscript(claimed, []NoteLine{{Speaker: SpeakerOwner, Text: "Oui"}}, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, claimed.Audio)); err == nil {
		t.Fatal("the recording stayed after it was written down")
	}
}

// An app that crashes never says it's done: the note goes ahead with what it has.
func TestRecordingAfterACrash(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	s.StartRecording("joes", "c1", "audio/webm")
	s.AppendRecording(u.AccountID, "c1", 0, []byte("sound"))
	if _, ok, _ := s.ClaimNote(); ok {
		t.Fatal("claimed too soon")
	}
	s.db.Exec("UPDATE note SET next_try = datetime('now', '-1 seconds')")
	if n, ok, _ := s.ClaimNote(); !ok || n.Kind != NoteCall {
		t.Fatalf("claim: %+v %v", n, ok)
	}
}

// A call that ended before its first piece has nothing to say.
func TestRecordingWithNoPieces(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	s.StartRecording("joes", "c1", "audio/webm")
	if err := s.FinishRecording(u.AccountID, "c1"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.Notes(u.AccountID); len(list) != 0 {
		t.Fatalf("notes: %+v", list)
	}
}

func TestRecordingLimits(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	s.StartRecording("joes", "big", "audio/webm")
	if err := s.AppendRecording(u.AccountID, "big", 0, make([]byte, MaxRecordingPiece+1)); !errors.Is(err, ErrRecordingFull) {
		t.Fatalf("a piece too big: %v", err)
	}
	s.db.Exec("UPDATE note SET audio_size = ? WHERE call_id = 'big'", MaxRecordingBytes)
	if err := s.AppendRecording(u.AccountID, "big", 0, []byte("x")); !errors.Is(err, ErrRecordingFull) {
		t.Fatalf("a recording too big: %v", err)
	}
	s.db.Exec("UPDATE note SET audio_size = 0 WHERE call_id = 'big'")
	s.ChargeNotes(u.AccountID, NoteMonthSeconds)
	s.db.Exec("UPDATE note SET created_at = datetime('now', '-5 seconds') WHERE call_id = 'big'")
	if err := s.AppendRecording(u.AccountID, "big", 0, []byte("x")); !errors.Is(err, ErrRecordingFull) {
		t.Fatalf("past the month: %v", err)
	}
	s.db.Exec("UPDATE note SET audio_size = ? WHERE call_id = 'big'", int64(MaxRecordingDisk))
	if _, err := s.StartRecording("joes", "next", "audio/webm"); !errors.Is(err, ErrRecordingFull) {
		t.Fatalf("the disk's share: %v", err)
	}
	if _, err := s.StartRecording("nope", "c9", "audio/webm"); !errors.Is(err, ErrRecordingFull) {
		t.Fatalf("disk first: %v", err)
	}
	s.db.Exec("UPDATE note SET audio_size = 0")
	s.SetTakeNotes(u.AccountID, false)
	if _, err := s.StartRecording("joes", "off", "audio/webm"); !errors.Is(err, ErrNotesOff) {
		t.Fatalf("switch off: %v", err)
	}
	s.SetTakeNotes(u.AccountID, true)
	s.SetPlan(u.AccountID, Basic)
	if _, err := s.StartRecording("joes", "basic", "audio/webm"); !errors.Is(err, ErrNotPremium) {
		t.Fatalf("Basic: %v", err)
	}
}

func TestSkipCallNote(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	if err := s.SkipCallNote("joes", "c1", NoteNotSetUp); err != nil {
		t.Fatal(err)
	}
	s.SkipCallNote("joes", "c1", NoteDayLimit)
	list, _ := s.Notes(u.AccountID)
	if len(list) != 1 || list[0].Status != NoteSkipped || list[0].Reason != NoteNotSetUp || list[0].Audio != "" {
		t.Fatalf("notes: %+v", list)
	}
	s.SetPlan(u.AccountID, Basic)
	if err := s.SkipCallNote("joes", "c2", NoteNotSetUp); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Notes(u.AccountID); len(list) != 1 {
		t.Fatalf("Basic gets no note: %+v", list)
	}
}

// A recording no note points at goes after a day, and one being made stays.
func TestSweepRecordings(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	n, _ := s.StartRecording("joes", "c1", "audio/webm")
	s.AppendRecording(u.AccountID, "c1", 0, []byte("sound"))
	mine := filepath.Join(s.Dir, n.Audio)
	lost := filepath.Join(s.Dir, "notes", "call-lost.webm")
	fresh := filepath.Join(s.Dir, "notes", "call-fresh.webm")
	os.WriteFile(lost, []byte("x"), 0o600)
	os.WriteFile(fresh, []byte("x"), 0o600)
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(lost, old, old)
	os.Chtimes(mine, old, old)
	if err := s.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lost); err == nil {
		t.Error("a lost recording stayed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a new one went")
	}
	if _, err := os.Stat(mine); err != nil {
		t.Error("a note's recording went")
	}
}

func TestRoomRecording(t *testing.T) {
	s := open(t)
	u := notesAccount(t, s, "joe@example.com", "joes")
	sue := notesAccount(t, s, "sue@example.com", "sues")
	if err := s.AddNotePerson("joes", "Early"); err != nil {
		t.Fatalf("no meeting yet: %v", err)
	}
	n, err := s.StartRoomRecording("joes", "audio/webm", []string{"Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != NoteRoom || !strings.HasPrefix(n.CallID, "room-") || !strings.HasPrefix(n.Audio, "notes/room-") || len(n.People) != 1 {
		t.Fatalf("note: %+v", n)
	}
	s.AddNotePerson("joes", "Bo")
	s.AddNotePerson("joes", "Ana")
	s.AddNotePerson("sues", "Not here")
	got, _ := s.NoteByID(u.AccountID, n.ID)
	if len(got.People) != 2 || got.People[0] != "Ana" || got.People[1] != "Bo" {
		t.Fatalf("people: %v", got.People)
	}
	for i := range maxNotePeople + 5 {
		s.AddNotePerson("joes", fmt.Sprintf("Guest %d", i))
	}
	if got, _ = s.NoteByID(u.AccountID, n.ID); len(got.People) != maxNotePeople {
		t.Fatalf("a robot's names: %d", len(got.People))
	}
	if err := s.AppendRecording(sue.AccountID, n.CallID, 0, []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("someone else's piece: %v", err)
	}
	if err := s.AppendRecording(u.AccountID, n.CallID, 0, []byte("x")); err != nil {
		t.Fatal(err)
	}
	s.FinishRecording(u.AccountID, n.CallID)
	s.ClaimNote()
	s.AddNotePerson("joes", "Late")
	if got, _ = s.NoteByID(u.AccountID, n.ID); slices.Contains(got.People, "Late") {
		t.Fatal("someone added after the meeting")
	}
	if err := s.SkipRoomNote("joes", NoteNotSetUp, []string{"Ana"}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Notes(u.AccountID)
	if len(list) != 2 || list[0].Reason != NoteNotSetUp || list[0].People[0] != "Ana" || list[0].Audio != "" {
		t.Fatalf("skipped: %+v", list[0])
	}
	s.SetPlan(u.AccountID, Basic)
	if _, err := s.StartRoomRecording("joes", "audio/webm", nil); !errors.Is(err, ErrNotPremium) {
		t.Fatalf("Basic: %v", err)
	}
	if err := s.SkipRoomNote("joes", NoteNotSetUp, nil); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.Notes(u.AccountID); len(list) != 2 {
		t.Fatalf("Basic gets no note: %d", len(list))
	}
}
