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

// A call's recording comes up in pieces from the owner's app (assets/js/recorder.js).
// Only the line's logged-in owner reaches these, and only while that call is on
// can one start; the pieces then belong to its note.

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
	if !s.Hub.Answered(l.Name, id) {
		http.NotFound(w, r)
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
	if reason != "" {
		if err := s.Store.SkipCallNote(l.Name, id, reason); err != nil {
			log.Printf("call %s: note: %v", id, err)
		}
		answer(false, reason)
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
		s.Store.SkipCallNote(l.Name, id, store.NoteDayLimit)
		answer(false, store.NoteDayLimit)
	case err != nil:
		serverError(w, r, err)
	default:
		answer(true, "")
	}
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
	err = s.Store.AppendRecording(u.AccountID, r.PathValue("id"), part, data)
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
	err := s.Store.FinishRecording(u.AccountID, r.PathValue("id"))
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
