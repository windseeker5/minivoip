// Package notes turns a saved recording into Premium's notes: a transcript from
// one service, then a summary and next actions from another. One worker does
// one note at a time in the background; its queue is the note table, so it
// picks up where it left off after a restart. Go tests use fakes of the two
// services (see Transcriber, Summarizer), so they never spend money.
package notes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"minivoip/store"
)

// Transcriber writes down what was said in a recording. channels names who is
// on each of its sound channels; with none, the service tells voices apart.
type Transcriber interface {
	Transcribe(ctx context.Context, file, contentType string, channels []string) (Transcript, error)
}

// Summarizer writes the note from a transcript, in the given language. What
// people said is untrusted: it can only ever end up in this note.
type Summarizer interface {
	Summarize(ctx context.Context, lines []store.NoteLine, language store.Language) (Summary, error)
}

type Transcript struct {
	Lines   []store.NoteLine
	Seconds int64 // the recording's real length, what the service bills
}

type Summary struct {
	Text        string
	NextActions []string
}

// ErrPermanent marks an error that trying again won't fix, like a recording the
// service can't read.
var ErrPermanent = errors.New("won't work on another try")

// CallChannels is who is on a call recording's two sound channels (recorder.js).
var CallChannels = []string{store.SpeakerOwner, store.SpeakerCaller}

// MaxTries is how many times a note is tried before it fails.
const MaxTries = 4

// DefaultDaySeconds is the ceiling across all accounts in a day (UTC), to protect
// Ken's bill: 20 hours of audio is about $7 of both services, up to $11 for calls
// (their two channels are billed apart).
const DefaultDaySeconds = 20 * 3600

// A recording says at most this many bytes a second, even in Safari's bigger
// format, so a file's size shows a length the browser may have played down.
const maxBytesPerSecond = 20000

type Worker struct {
	Store       *store.Store
	Transcriber Transcriber // nil, like Summarizer, when its key isn't set
	Summarizer  Summarizer
	DaySeconds  int64
	// Backoff says how long to wait before another try, after tries so far.
	Backoff func(tries int) time.Duration

	wake chan struct{}
}

func NewWorker(st *store.Store, t Transcriber, s Summarizer) *Worker {
	return &Worker{Store: st, Transcriber: t, Summarizer: s, DaySeconds: DefaultDaySeconds, Backoff: backoff, wake: make(chan struct{}, 1)}
}

func backoff(tries int) time.Duration {
	switch tries {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	}
	return 30 * time.Minute
}

// SetUp says whether both services have their keys.
func (w *Worker) SetUp() bool { return w.Transcriber != nil && w.Summarizer != nil }

// Wake tells the worker there's a new note, instead of waiting for its next look.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run works through the queue until ctx ends.
func (w *Worker) Run(ctx context.Context) {
	if err := w.Store.ReleaseNotes(); err != nil {
		log.Printf("notes: %v", err)
	}
	look := time.NewTicker(30 * time.Second) // for notes waiting for their next try
	defer look.Stop()
	for {
		if w.Next(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-look.C:
		}
	}
}

// Next does the next note whose turn has come, and says if there was one.
func (w *Worker) Next(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	n, ok, err := w.Store.ClaimNote()
	if err != nil {
		log.Printf("notes: %v", err)
		return false
	}
	if ok {
		w.do(ctx, n)
	}
	return ok
}

func (w *Worker) do(ctx context.Context, n store.Note) {
	if !w.SetUp() {
		w.end(n, store.NoteSkipped, store.NoteNotSetUp)
		return
	}
	// Past its last try only if the server kept stopping in the middle of it.
	if n.Tries > MaxTries {
		reason := store.NoteSummarize
		if n.Audio != "" {
			reason = store.NoteTranscribe
		}
		w.end(n, store.NoteFailed, reason)
		return
	}
	// The plan is checked again: the account can have changed since the note was queued.
	if b, err := w.Store.Billing(n.AccountID); err != nil {
		w.tryAgain(ctx, n, store.NoteTranscribe, err)
		return
	} else if b.Plan != store.Premium {
		w.end(n, store.NoteSkipped, store.NoteNotPremium)
		return
	}
	// Once transcribed there's no recording left, and another try only pays for the summary.
	if n.Audio != "" {
		var done bool
		if n, done = w.transcribe(ctx, n); done {
			return
		}
	}
	s, err := w.Summarizer.Summarize(ctx, n.Transcript, n.Language)
	if err != nil {
		w.tryAgain(ctx, n, store.NoteSummarize, err)
		return
	}
	if err := w.Store.FinishNote(n.ID, s.Text, s.NextActions); err != nil {
		log.Printf("notes: note %d: %v", n.ID, err)
	}
}

// transcribe gets the note's transcript, or ends its work for now (done).
func (w *Worker) transcribe(ctx context.Context, n store.Note) (store.Note, bool) {
	path := w.Store.NoteAudioPath(n)
	info, err := os.Stat(path)
	if err != nil {
		w.end(n, store.NoteFailed, store.NoteNoAudio)
		return n, true
	}
	if reason, err := w.limit(n.AccountID, max(n.Seconds, info.Size()/maxBytesPerSecond)); err != nil {
		w.tryAgain(ctx, n, store.NoteTranscribe, err)
		return n, true
	} else if reason != "" {
		w.end(n, store.NoteSkipped, reason)
		return n, true
	}
	var channels []string
	if n.Kind == store.NoteCall {
		channels = CallChannels
	}
	t, err := w.Transcriber.Transcribe(ctx, path, n.AudioType, channels)
	if err != nil {
		w.tryAgain(ctx, n, store.NoteTranscribe, err)
		return n, true
	}
	// A call's clock counts the quiet at its end, which the words don't show.
	seconds := t.Seconds
	if n.Kind == store.NoteCall {
		seconds = max(seconds, n.Seconds)
	}
	if err := w.Store.SaveTranscript(n, t.Lines, seconds); err != nil {
		w.tryAgain(ctx, n, store.NoteTranscribe, err)
		return n, true
	}
	n.Transcript, n.Seconds = t.Lines, seconds
	if len(t.Lines) == 0 {
		w.end(n, store.NoteSkipped, store.NoteSilent) // nothing to summarize, nothing to pay for
		return n, true
	}
	n.Audio = ""
	return n, false
}

// limit says which limit stops this much audio before any money is spent, or "".
func (w *Worker) limit(accountID, seconds int64) (string, error) {
	month, err := w.Store.NoteSecondsThisMonth(accountID)
	if err != nil {
		return "", err
	}
	if month+seconds > store.NoteMonthSeconds {
		return store.NoteMonthLimit, nil
	}
	today, err := w.Store.NoteSecondsToday()
	if err != nil {
		return "", err
	}
	if today+seconds > w.DaySeconds {
		return store.NoteDayLimit, nil
	}
	return "", nil
}

// Refuse says why a recording shouldn't start for this account, or "": so a call
// isn't recorded for a note that can't be written.
func (w *Worker) Refuse(accountID int64) string {
	if !w.SetUp() {
		return store.NoteNotSetUp
	}
	reason, err := w.limit(accountID, 1)
	if err != nil {
		log.Printf("notes: %v", err)
		return store.NoteTranscribe
	}
	return reason
}

func (w *Worker) end(n store.Note, status store.NoteStatus, reason string) {
	if err := w.Store.EndNote(n, status, reason); err != nil {
		log.Printf("notes: note %d: %v", n.ID, err)
	}
}

// tryAgain waits for another try, or fails the note after the last one. A server
// stopping isn't the note's fault: it stays working, and Run puts it back.
func (w *Worker) tryAgain(ctx context.Context, n store.Note, reason string, err error) {
	if ctx.Err() != nil {
		return
	}
	// Never the transcript itself: the log only says which note and what went wrong.
	log.Printf("notes: note %d, try %d: %v", n.ID, n.Tries, err)
	if errors.Is(err, ErrPermanent) || n.Tries >= MaxTries {
		w.end(n, store.NoteFailed, reason)
		return
	}
	if err := w.Store.RetryNote(n.ID, int(w.Backoff(n.Tries)/time.Second)); err != nil {
		log.Printf("notes: note %d: %v", n.ID, err)
	}
}

// FromEnv is the real services, for the keys that are set; a missing key leaves
// its service nil, and new notes say they aren't set up yet.
func FromEnv() (Transcriber, Summarizer) {
	var t Transcriber
	var s Summarizer
	if key := os.Getenv("ELEVENLABS_API_KEY"); key != "" {
		t = &Scribe{Key: key}
	}
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		s = &Claude{Key: key}
	}
	return t, s
}

// client talks to the services. It follows no redirect, so a key can't travel to
// another address.
var client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// statusError is what a service answered instead of a result. Its own words are
// kept short, and never hold a transcript: they only ever say what was wrong.
func statusError(service string, status int, body []byte) error {
	if len(body) > 300 {
		body = body[:300]
	}
	err := fmt.Errorf("%s: status %d: %s", service, status, body)
	// The request itself is wrong, and will be next time too. A bad key (401, 403)
	// and a busy service (429, 5xx) can get better.
	if status == 400 || status == 404 || status == 413 || status == 415 || status == 422 {
		return fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return err
}
