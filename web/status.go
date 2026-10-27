package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"minivoip/lines"
	"minivoip/pages"
	"minivoip/store"
)

// here says whether a line's owner can talk now: the Available switch is on,
// a choice rings, and a phone can ring (an open phone app, or one that gets
// pushes). Otherwise the Call button says "Leave us a message"; the menu
// still answers.
func (s *Server) here(l lines.Line) bool {
	if !store.IsAvailable(l.AwayUntil) || !l.Rings() {
		return false
	}
	if s.Hub.OwnersOnline(l.Name) > 0 {
		return true
	}
	d, _ := s.Store.DevicesForLine(l.Name)
	return len(d) > 0
}

// status is /api/status/<line>, read by the Call button on any website
// (w.js) and by the call page while it's open.
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	l, err := s.Store.LineByName(r.PathValue("line"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pages.NewPresence(l, s.here(l)))
}
