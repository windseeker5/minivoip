package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Notes are Premium's written record of a call, a meeting room or a voicemail:
// the transcript, a summary and the next actions. The notes package does the
// work one note at a time; this file only keeps them.

type NoteKind string

const (
	NoteVoicemail NoteKind = "voicemail"
	NoteCall      NoteKind = "call"
	NoteRoom      NoteKind = "room"
)

type NoteStatus string

const (
	NoteWaiting NoteStatus = "waiting"
	NoteWorking NoteStatus = "working"
	NoteDone    NoteStatus = "done"
	NoteFailed  NoteStatus = "failed"
	NoteSkipped NoteStatus = "skipped"
)

// Why a note was skipped or failed (note.reason); the dashboard words them.
const (
	NoteNotSetUp   = "not_set_up"
	NoteNotPremium = "not_premium"
	NoteMonthLimit = "month_limit"
	NoteDayLimit   = "day_limit"
	NoteSilent     = "silent"
	NoteNoAudio    = "no_audio"
	NoteTranscribe = "transcribe"
	NoteSummarize  = "summarize"
)

// NoteMonthSeconds is how much audio an account's notes can cover in a calendar
// month (UTC).
const NoteMonthSeconds = 10 * 3600

// MinNoteMS is the shortest voicemail that gets a note, and
// MaxVoicemailNotesPerDay how many a line's voicemails get in a day.
const (
	MinNoteMS               = 2000
	MaxVoicemailNotesPerDay = 30
)

// How long the notes and their hours are kept.
const (
	noteDays      = 365
	noteUsageDays = 62
)

// What a summary can hold, whatever the service sends back.
const (
	maxNoteSummary = 20000
	maxNoteActions = 30
	maxNoteAction  = 1000
	maxNotePeople  = 30
)

var (
	ErrNotPremium = errors.New("notes are for Premium accounts")
	ErrNotesOff   = errors.New("this account's Take notes switch is off")
)

// A call's speakers are known, each on a sound channel of its own.
const (
	SpeakerOwner  = "owner"
	SpeakerCaller = "caller"
)

// NoteLine is one speaker's turn in a transcript, kept word for word as spoken.
type NoteLine struct {
	Speaker string  `json:"speaker"`
	Start   float64 `json:"start"` // seconds from the start of the recording
	Text    string  `json:"text"`
}

type Note struct {
	ID          int64
	AccountID   int64
	LineID      int64
	Kind        NoteKind
	Status      NoteStatus
	Reason      string
	Language    Language // what the summary is written in
	Audio       string   // file under the data folder; "" once transcribed
	AudioType   string
	Seconds     int64 // the recording's length: first as told, then as measured
	Transcript  []NoteLine
	Summary     string
	NextActions []string
	Tries       int
	People      []string // a meeting's guests, as they named themselves
	CallID      string   // the call or meeting it records
	VoicemailID int64    // a voicemail's message; 0 if none or gone
	CreatedAt   string   // UTC "YYYY-MM-DD HH:MM:SS"
	DoneAt      string
}

const noteColumns = `id, account_id, line_id, kind, status, reason, language, audio, audio_type, seconds,
	transcript, summary, next_actions, tries, people, call_id, COALESCE(voicemail_id, 0), created_at, COALESCE(done_at, '')`

// noteListColumns leaves the transcripts out: a list doesn't show them, and a
// year of meetings is a lot of words.
var noteListColumns = strings.Replace(noteColumns, "transcript,", "'',", 1)

// MaxNotesListed is how many notes the Notes tab lists.
const MaxNotesListed = 500

func scanNote(row interface{ Scan(...any) error }) (Note, error) {
	var n Note
	var transcript, actions, people string
	err := row.Scan(&n.ID, &n.AccountID, &n.LineID, &n.Kind, &n.Status, &n.Reason, &n.Language, &n.Audio, &n.AudioType, &n.Seconds,
		&transcript, &n.Summary, &actions, &n.Tries, &people, &n.CallID, &n.VoicemailID, &n.CreatedAt, &n.DoneAt)
	if err != nil {
		return Note{}, err
	}
	if transcript != "" {
		if err := json.Unmarshal([]byte(transcript), &n.Transcript); err != nil {
			return Note{}, err
		}
	}
	if actions != "" {
		if err := json.Unmarshal([]byte(actions), &n.NextActions); err != nil {
			return Note{}, err
		}
	}
	if people != "" {
		if err := json.Unmarshal([]byte(people), &n.People); err != nil {
			return Note{}, err
		}
	}
	return n, nil
}

// AddNote queues a note for a recording already saved under the data folder.
// seconds is its length as far as anyone knows yet. Only Premium accounts get
// notes, and the note takes the account's language of the moment.
func (s *Store) AddNote(lineName string, kind NoteKind, audio, audioType string, seconds int64) (int64, error) {
	return s.addNote(lineName, newNote{kind: kind, audio: audio, audioType: audioType, seconds: seconds, status: NoteWaiting})
}

// newNote is one note to add. wait holds its first try back, in seconds.
type newNote struct {
	kind      NoteKind
	audio     string
	audioType string
	seconds   int64
	voicemail any // its message's id, or nil
	callID    string
	status    NoteStatus
	reason    string
	wait      int
	people    []string
}

func (s *Store) addNote(lineName string, n newNote) (int64, error) {
	// Only a note closed from the start has no recording.
	if (n.audio != "" || n.status == NoteWaiting) && !filepath.IsLocal(n.audio) {
		return 0, fmt.Errorf("note audio outside the data folder: %q", n.audio)
	}
	var lineID, accountID int64
	var plan Plan
	var language Language
	var takeNotes bool
	err := s.db.QueryRow(`SELECT l.id, a.id, a.plan, a.language, a.take_notes FROM line l JOIN account a ON a.id = l.account_id
		WHERE l.name = ?`, lineName).Scan(&lineID, &accountID, &plan, &language, &takeNotes)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if plan != Premium {
		return 0, ErrNotPremium
	}
	if !takeNotes {
		return 0, ErrNotesOff
	}
	people := ""
	if len(n.people) > 0 {
		b, err := json.Marshal(n.people)
		if err != nil {
			return 0, err
		}
		people = string(b)
	}
	res, err := s.db.Exec(`INSERT INTO note (account_id, line_id, kind, language, audio, audio_type, seconds, voicemail_id, call_id, status, reason, next_try, people)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now', ?), ?)`,
		accountID, lineID, n.kind, language, n.audio, n.audioType, n.seconds, n.voicemail, n.callID, n.status, n.reason, fmt.Sprintf("+%d seconds", n.wait), people)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// NoteVoicemail queues a note for a saved message, if its account gets notes.
// A message shorter than this says nothing worth paying for, and gets none. The
// length comes from the browser, but playing it down only loses the note, and
// playing it up changes nothing: the worker also goes by the file's size. Past
// a line's share of a day, the message gets a note saying why it has no words:
// a robot leaving messages can't use up the owner's hours or everyone's day.
func (s *Store) NoteVoicemail(v Voicemail) (bool, error) {
	if v.DurationMS < MinNoteMS {
		return false, nil
	}
	var line, file string
	var today int
	err := s.db.QueryRow(`SELECT l.name, v.file, (SELECT count(*) FROM note n WHERE n.line_id = v.line_id AND n.kind = 'voicemail'
			AND n.created_at > datetime('now', '-1 day'))
		FROM voicemail v JOIN line l ON l.id = v.line_id WHERE v.id = ?`, v.ID).Scan(&line, &file, &today)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	audio, status, reason := filepath.Join(filepath.Base(s.voicemailDir()), file), NoteWaiting, ""
	if today >= MaxVoicemailNotesPerDay {
		audio, status, reason = "", NoteSkipped, NoteDayLimit
	}
	_, err = s.addNote(line, newNote{kind: NoteVoicemail, audio: audio, audioType: v.ContentType, seconds: v.Seconds(), voicemail: v.ID, status: status, reason: reason})
	if errors.Is(err, ErrNotPremium) || errors.Is(err, ErrNotesOff) {
		return false, nil
	}
	return err == nil, err
}

// A call's recording arrives in pieces from the owner's app (web/recording.go).
const (
	// MaxRecordingPiece caps one piece: 30 seconds, whatever the browser's format.
	MaxRecordingPiece = 4 << 20
	// MaxRecordingBytes caps one recording: about the month's 10 hours.
	MaxRecordingBytes = 300 << 20
	// MaxRecordingDisk caps every recording waiting to be written down.
	MaxRecordingDisk = 20 << 30
	// recordingGrace is how long, in seconds, a note waits for its next piece
	// before it goes ahead with what it has: an app that crashed loses little.
	recordingGrace = 120
)

var (
	ErrRecordingStale = errors.New("this recording isn't taking pieces any more")
	ErrRecordingFull  = errors.New("this recording is full")
)

func (s *Store) notesDir() string { return filepath.Join(s.Dir, "notes") }

// StartRecording opens the note of a call being answered now, and says where
// its pieces go. A call has one note, so a second start finds the first.
func (s *Store) StartRecording(lineName, callID, audioType string) (Note, error) {
	return s.startRecording(lineName, NoteCall, callID, audioType, nil)
}

// StartRoomRecording opens the note of a meeting under way, with the guests in
// the room so far. A meeting has no id of its own, so it gets one.
func (s *Store) StartRoomRecording(lineName, audioType string, people []string) (Note, error) {
	id, err := newRecordingID()
	if err != nil {
		return Note{}, err
	}
	return s.startRecording(lineName, NoteRoom, "room-"+id, audioType, people)
}

func newRecordingID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) startRecording(lineName string, kind NoteKind, callID, audioType string, people []string) (Note, error) {
	var disk int64
	if err := s.db.QueryRow("SELECT COALESCE(sum(audio_size), 0) FROM note WHERE audio <> '' AND kind <> 'voicemail'").Scan(&disk); err != nil {
		return Note{}, err
	}
	if disk >= MaxRecordingDisk {
		return Note{}, ErrRecordingFull
	}
	if n, err := s.noteByCall(callID); err == nil {
		return n, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Note{}, err
	}
	id, err := newRecordingID()
	if err != nil {
		return Note{}, err
	}
	audio := filepath.Join(filepath.Base(s.notesDir()), string(kind)+"-"+id+extFor(audioType))
	_, err = s.addNote(lineName, newNote{kind: kind, audio: audio, audioType: audioType, callID: callID, status: NoteWaiting, wait: recordingGrace, people: people})
	if err != nil {
		return Note{}, err
	}
	return s.noteByCall(callID)
}

// SkipCallNote leaves a note saying why a call isn't being recorded.
func (s *Store) SkipCallNote(lineName, callID, reason string) error {
	if _, err := s.noteByCall(callID); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	_, err := s.addNote(lineName, newNote{kind: NoteCall, callID: callID, status: NoteSkipped, reason: reason})
	if errors.Is(err, ErrNotPremium) || errors.Is(err, ErrNotesOff) {
		return nil
	}
	return err
}

// SkipRoomNote leaves a note saying why a meeting isn't being recorded.
func (s *Store) SkipRoomNote(lineName, reason string, people []string) error {
	id, err := newRecordingID()
	if err != nil {
		return err
	}
	_, err = s.addNote(lineName, newNote{kind: NoteRoom, callID: "room-" + id, status: NoteSkipped, reason: reason, people: people})
	if errors.Is(err, ErrNotPremium) || errors.Is(err, ErrNotesOff) {
		return nil
	}
	return err
}

// AddNotePerson adds a guest to the line's meeting being recorded now, if any.
func (s *Store) AddNotePerson(lineName, name string) error {
	var id int64
	var raw string
	err := s.db.QueryRow(`SELECT n.id, n.people FROM note n JOIN line l ON l.id = n.line_id
		WHERE l.name = ? AND n.kind = 'room' AND n.status = 'waiting' ORDER BY n.id DESC LIMIT 1`, lineName).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var people []string
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &people); err != nil {
			return err
		}
	}
	if len(people) >= maxNotePeople || slices.Contains(people, name) {
		return nil
	}
	b, err := json.Marshal(append(people, name))
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE note SET people = ? WHERE id = ?", string(b), id)
	return err
}

func (s *Store) noteByCall(callID string) (Note, error) {
	n, err := scanNote(s.db.QueryRow("SELECT "+noteColumns+" FROM note WHERE call_id = ?", callID))
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return n, err
}

// AppendRecording adds piece number part (from 0) to a call's recording. Pieces
// come one at a time; one sent again after a lost answer is already in. A
// recording past the account's month, its own size, or already being written
// down takes no more, and the app stops.
func (s *Store) AppendRecording(accountID int64, callID string, part int, data []byte) error {
	s.recording.Lock()
	defer s.recording.Unlock()
	var id, size, month int64
	var parts int
	var audio string
	var status NoteStatus
	err := s.db.QueryRow(`SELECT id, parts, audio_size, audio, status,
			CAST(strftime('%s', 'now') AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER)
				+ (SELECT COALESCE(sum(u.seconds), 0) FROM note_usage u WHERE u.account_id = note.account_id AND u.day >= date('now', 'start of month'))
		FROM note WHERE call_id = ? AND account_id = ?`, callID, accountID).Scan(&id, &parts, &size, &audio, &status, &month)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != NoteWaiting || audio == "" || part > parts {
		return ErrRecordingStale
	}
	if part < parts {
		return nil
	}
	if len(data) > MaxRecordingPiece || size+int64(len(data)) > MaxRecordingBytes || month > NoteMonthSeconds {
		return ErrRecordingFull
	}
	if err := os.MkdirAll(s.notesDir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, audio), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE note SET parts = parts + 1, audio_size = audio_size + ?,
			seconds = CAST(strftime('%s', 'now') AS INTEGER) - CAST(strftime('%s', created_at) AS INTEGER),
			next_try = datetime('now', ?)
		WHERE id = ? AND status = 'waiting'`, len(data), fmt.Sprintf("+%d seconds", recordingGrace), id)
	return err
}

// FinishRecording says the last piece is in: the note's turn comes now. A call
// that ended before any piece arrived has nothing to say, and goes.
func (s *Store) FinishRecording(accountID int64, callID string) error {
	res, err := s.db.Exec("DELETE FROM note WHERE call_id = ? AND account_id = ? AND status = 'waiting' AND parts = 0", callID, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	res, err = s.db.Exec("UPDATE note SET next_try = datetime('now') WHERE call_id = ? AND account_id = ? AND status = 'waiting'", callID, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRecordingStale
	}
	return nil
}

// ClaimNote takes the oldest note whose turn has come and marks it working.
func (s *Store) ClaimNote() (Note, bool, error) {
	n, err := scanNote(s.db.QueryRow(`UPDATE note SET status = 'working', tries = tries + 1
		WHERE id = (SELECT id FROM note WHERE status = 'waiting' AND next_try <= datetime('now') ORDER BY id LIMIT 1)
		RETURNING ` + noteColumns))
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, false, nil
	}
	return n, err == nil, err
}

// ReleaseNotes puts back the notes a stopped server was working on. The try
// still counts, so a note can't spend money without end.
func (s *Store) ReleaseNotes() error {
	_, err := s.db.Exec("UPDATE note SET status = 'waiting' WHERE status = 'working'")
	return err
}

// NoteAudioPath is where a note's recording is.
func (s *Store) NoteAudioPath(n Note) string { return filepath.Join(s.Dir, n.Audio) }

// dropNoteAudio deletes a call's or a room's recording. A voicemail's stays: it
// is the owner's message, with its own life.
func (s *Store) dropNoteAudio(kind NoteKind, audio string) {
	if kind != NoteVoicemail && audio != "" {
		os.Remove(filepath.Join(s.Dir, audio))
	}
}

// SaveTranscript keeps a note's transcript and its measured length, counts the
// hours, and deletes the recording of a call or a room. The hours count even if
// the owner deleted the note in the meantime: the service was paid for.
func (s *Store) SaveTranscript(n Note, lines []NoteLine, seconds int64) error {
	b, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE note SET transcript = ?, seconds = ?, audio = '' WHERE id = ?", string(b), seconds, n.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(chargeNotesSQL, n.AccountID, seconds); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.dropNoteAudio(n.Kind, n.Audio)
	return nil
}

// FinishNote keeps the summary and next actions, cut to what a page can hold.
func (s *Store) FinishNote(id int64, summary string, actions []string) error {
	if r := []rune(summary); len(r) > maxNoteSummary {
		summary = string(r[:maxNoteSummary])
	}
	if len(actions) > maxNoteActions {
		actions = actions[:maxNoteActions]
	}
	for i, a := range actions {
		if r := []rune(a); len(r) > maxNoteAction {
			actions[i] = string(r[:maxNoteAction])
		}
	}
	b, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE note SET status = 'done', reason = '', summary = ?, next_actions = ?, done_at = datetime('now') WHERE id = ? AND status = 'working'",
		summary, string(b), id)
	return err
}

// RetryNote puts a note back to wait for its next try.
func (s *Store) RetryNote(id int64, afterSeconds int) error {
	_, err := s.db.Exec("UPDATE note SET status = 'waiting', next_try = datetime('now', ?) WHERE id = ? AND status = 'working'",
		fmt.Sprintf("+%d seconds", afterSeconds), id)
	return err
}

// EndNote closes a note that won't be finished (status skipped or failed), says
// why, and deletes the recording of a call or a room. A transcript it already
// has stays.
func (s *Store) EndNote(n Note, status NoteStatus, reason string) error {
	if _, err := s.db.Exec("UPDATE note SET status = ?, reason = ?, audio = '', done_at = datetime('now') WHERE id = ? AND status = 'working'",
		status, reason, n.ID); err != nil {
		return err
	}
	s.dropNoteAudio(n.Kind, n.Audio)
	return nil
}

// TakeNotes says whether the account's "Take notes" switch is on.
func (s *Store) TakeNotes(accountID int64) (bool, error) {
	var on bool
	err := s.db.QueryRow("SELECT take_notes FROM account WHERE id = ?", accountID).Scan(&on)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return on, err
}

func (s *Store) SetTakeNotes(accountID int64, on bool) error {
	res, err := s.db.Exec("UPDATE account SET take_notes = ? WHERE id = ?", on, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Notes lists an account's notes, newest first, without their transcripts.
func (s *Store) Notes(accountID int64) ([]Note, error) {
	rows, err := s.db.Query("SELECT "+noteListColumns+" FROM note WHERE account_id = ? ORDER BY created_at DESC, id DESC LIMIT ?", accountID, MaxNotesListed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// NoteByID returns one of the account's notes, or ErrNotFound.
func (s *Store) NoteByID(accountID, id int64) (Note, error) {
	n, err := scanNote(s.db.QueryRow("SELECT "+noteColumns+" FROM note WHERE id = ? AND account_id = ?", id, accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return n, err
}

// DeleteNote removes one of the account's notes. Its hours stay counted.
func (s *Store) DeleteNote(accountID, id int64) error {
	n, err := s.NoteByID(accountID, id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec("DELETE FROM note WHERE id = ?", id); err != nil {
		return err
	}
	s.dropNoteAudio(n.Kind, n.Audio)
	return nil
}

const chargeNotesSQL = `INSERT INTO note_usage (account_id, day, seconds) VALUES (?, date('now'), ?)
	ON CONFLICT (account_id, day) DO UPDATE SET seconds = seconds + excluded.seconds`

// ChargeNotes counts audio against the account's month and everyone's day.
func (s *Store) ChargeNotes(accountID, seconds int64) error {
	_, err := s.db.Exec(chargeNotesSQL, accountID, seconds)
	return err
}

// NoteSecondsThisMonth is how much audio the account's notes covered this month.
func (s *Store) NoteSecondsThisMonth(accountID int64) (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COALESCE(sum(seconds), 0) FROM note_usage
		WHERE account_id = ? AND day >= date('now', 'start of month')`, accountID).Scan(&n)
	return n, err
}

// NoteSecondsToday is how much audio all accounts' notes covered today.
func (s *Store) NoteSecondsToday() (int64, error) {
	var n int64
	err := s.db.QueryRow("SELECT COALESCE(sum(seconds), 0) FROM note_usage WHERE day = date('now')").Scan(&n)
	return n, err
}

// cleanupNotes forgets notes after a year, and their hours once no limit needs them.
func (s *Store) cleanupNotes() error {
	rows, err := s.db.Query("SELECT kind, audio FROM note WHERE created_at < datetime('now', ?)", fmt.Sprintf("-%d days", noteDays))
	if err != nil {
		return err
	}
	type old struct {
		kind  NoteKind
		audio string
	}
	var list []old
	for rows.Next() {
		var o old
		if err := rows.Scan(&o.kind, &o.audio); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := s.db.Exec("DELETE FROM note WHERE created_at < datetime('now', ?)", fmt.Sprintf("-%d days", noteDays)); err != nil {
		return err
	}
	for _, o := range list {
		s.dropNoteAudio(o.kind, o.audio)
	}
	if _, err = s.db.Exec("DELETE FROM note_usage WHERE day < date('now', ?)", fmt.Sprintf("-%d days", noteUsageDays)); err != nil {
		return err
	}
	return s.sweepRecordings()
}

// sweepRecordings deletes recordings no note points at (an account that went,
// say), leaving a day for one being made.
func (s *Store) sweepRecordings() error {
	files, err := os.ReadDir(s.notesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		info, err := f.Info()
		if err != nil || time.Since(info.ModTime()) < 24*time.Hour {
			continue
		}
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM note WHERE audio = ?", filepath.Join(filepath.Base(s.notesDir()), f.Name())).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			os.Remove(filepath.Join(s.notesDir(), f.Name()))
		}
	}
	return nil
}
