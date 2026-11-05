package web

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"minivoip/store"
)

// A call's or a meeting's recording comes up in pieces from the owner's browser
// (assets/js/recorder.js). Only the line's logged-in owner reaches these, and only
// while that call or meeting is on can one start; the pieces then belong to its note.

// recordingStart opens the note of the call on now, and says whether to record:
// not past a limit, nor without keys. Then the note says why, for the Notes tab.
func (s *Server) recordingStart(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	id := r.PathValue("id")
	on, told := s.Hub.Answered(l.Name, id)
	if !on {
		http.NotFound(w, r)
		return
	}
	if !s.Limits.Recordings.Take(strconv.FormatInt(u.AccountID, 10)) {
		http.Error(w, "Too many recordings. Please try again later.", http.StatusTooManyRequests)
		return
	}
	answer := func(record bool, reason string) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"record": record, "reason": reason})
	}
	if !l.Notes {
		answer(false, "")
		return
	}
	reason := store.NoteNotSetUp
	if s.RefuseNotes != nil {
		reason = s.RefuseNotes(u.AccountID)
	}
	// The caller's page loaded before notes were on, so it didn't tell them.
	if reason == "" && !told {
		reason = store.NoteNotTold
	}
	if reason != "" {
		if err := s.Store.SkipCallNote(l.Name, id, reason); err != nil {
			log.Printf("call %s: note: %v", id, err)
		}
		answer(false, reason)
		return
	}
	// A browser that can't record says so with no type: the note tells the owner.
	if r.URL.Query().Get("type") == "" {
		if err := s.Store.SkipCallNote(l.Name, id, store.NoteNoRecorder); err != nil {
			log.Printf("call %s: note: %v", id, err)
		}
		answer(false, store.NoteNoRecorder)
		return
	}
	audioType := store.AudioType(r.URL.Query().Get("type"))
	if audioType == "" {
		http.Error(w, "That isn't a sound recording.", http.StatusUnsupportedMediaType)
		return
	}
	_, err = s.Store.StartRecording(l.Name, id, audioType)
	switch {
	case errors.Is(err, store.ErrNotPremium), errors.Is(err, store.ErrNotesOff):
		answer(false, "")
	case errors.Is(err, store.ErrRecordingFull):
		s.Store.SkipCallNote(l.Name, id, store.NoteBusy)
		answer(false, store.NoteBusy)
	case err != nil:
		serverError(w, r, err)
	default:
		answer(true, "")
	}
}

// roomRecordingStart opens the note of the meeting on now, in the host's browser.
// It answers with where the pieces go: a meeting has no id until now.
func (s *Server) roomRecordingStart(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) || err == nil && l.Name != r.PathValue("line") {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	guests, on, told := s.Rooms.Meeting(l.Name)
	if !on {
		http.NotFound(w, r)
		return
	}
	if !s.Limits.Recordings.Take(strconv.FormatInt(u.AccountID, 10)) {
		http.Error(w, "Too many recordings. Please try again later.", http.StatusTooManyRequests)
		return
	}
	answer := func(record bool, reason, url string) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"record": record, "reason": reason, "url": url})
	}
	if !l.Notes {
		answer(false, "", "")
		return
	}
	reason := store.NoteNotSetUp
	if s.RefuseNotes != nil {
		reason = s.RefuseNotes(u.AccountID)
	}
	if reason == "" && !told {
		reason = store.NoteNotTold
	}
	if reason != "" {
		if err := s.Store.SkipRoomNote(l.Name, reason, guests); err != nil {
			log.Printf("room %s: note: %v", l.Name, err)
		}
		answer(false, reason, "")
		return
	}
	if r.URL.Query().Get("type") == "" {
		if err := s.Store.SkipRoomNote(l.Name, store.NoteNoRecorder, guests); err != nil {
			log.Printf("room %s: note: %v", l.Name, err)
		}
		answer(false, store.NoteNoRecorder, "")
		return
	}
	audioType := store.AudioType(r.URL.Query().Get("type"))
	if audioType == "" {
		http.Error(w, "That isn't a sound recording.", http.StatusUnsupportedMediaType)
		return
	}
	n, err := s.Store.StartRoomRecording(l.Name, audioType, guests)
	switch {
	case errors.Is(err, store.ErrNotPremium), errors.Is(err, store.ErrNotesOff):
		answer(false, "", "")
	case errors.Is(err, store.ErrRecordingFull):
		s.Store.SkipRoomNote(l.Name, store.NoteBusy, guests)
		answer(false, store.NoteBusy, "")
	case err != nil:
		serverError(w, r, err)
	default:
		answer(true, "", "/app/calls/"+n.CallID+"/recording")
	}
}

// missedSeconds is how much of the recording its browser reckons it didn't catch.
func missedSeconds(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get("missed"), 10, 64)
	return n
}

// recordingPiece takes the next piece. 409 and 413 tell the app to stop.
func (s *Server) recordingPiece(w http.ResponseWriter, r *http.Request, u store.User) {
	part, err := strconv.Atoi(r.PathValue("part"))
	if err != nil || part < 0 {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxRecordingPiece)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "This piece is too big.", http.StatusRequestEntityTooLarge)
		return
	}
	err = s.Store.AppendRecording(u.AccountID, r.PathValue("id"), part, data, missedSeconds(r))
	switch {
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, store.ErrRecordingStale):
		http.Error(w, "This recording is closed.", http.StatusConflict)
	case errors.Is(err, store.ErrRecordingFull):
		http.Error(w, "This recording is full.", http.StatusRequestEntityTooLarge)
	case err != nil:
		serverError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// recordingDone says the last piece is in, so the note's turn comes now.
func (s *Server) recordingDone(w http.ResponseWriter, r *http.Request, u store.User) {
	err := s.Store.FinishRecording(u.AccountID, r.PathValue("id"), missedSeconds(r))
	if errors.Is(err, store.ErrRecordingStale) {
		http.Error(w, "This recording is closed.", http.StatusConflict)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	if s.OnNote != nil {
		s.OnNote()
	}
	w.WriteHeader(http.StatusNoContent)
}
