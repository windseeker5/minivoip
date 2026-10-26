package web

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"minivoip/calls"
	"minivoip/pages"
	"minivoip/store"
)

// historySize is how many calls the phone app lists.
const historySize = 100

// phoneApp is the owner's app (/app): calls, voicemail, menu and settings,
// on the phone and the computer. phone.js keeps it connected and rings.
func (s *Server) phoneApp(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	history, err := s.Store.CallHistory(u.AccountID, historySize)
	if err != nil {
		serverError(w, r, err)
		return
	}
	list, err := s.Store.Voicemails(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	key := ""
	if s.Push != nil {
		key = s.Push.PublicKey()
	}
	render(w, r, pages.Phone(pages.AppData{
		Line:        l,
		Email:       u.Email,
		History:     history,
		Voicemails:  list,
		VapidKey:    key,
		CallURL:     strings.TrimPrefix(strings.TrimPrefix(baseURL(r), "https://"), "http://") + "/call/" + l.Name,
		RoomURL:     baseURL(r) + "/room/" + l.Name,
		RoomWaiting: s.Rooms.Waiting(l.Name),
		Devices:     s.Store.DeviceCount(u.AccountID),
		Android:     androidLatest(),
		RingSeconds: int(s.Hub.RingTimeout / time.Second),
	}))
}

// callsFragment is the call list alone, for the phone app to refresh.
func (s *Server) callsFragment(w http.ResponseWriter, r *http.Request, u store.User) {
	history, err := s.Store.CallHistory(u.AccountID, historySize)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.AppCalls(history))
}

// setAvailable is the app's Available switch: mode "on", "pause" (one hour)
// or "off" (until turned back on). It answers with the new switch.
func (s *Server) setAvailable(w http.ResponseWriter, r *http.Request, u store.User) {
	until := ""
	switch r.FormValue("mode") {
	case "on":
	case "pause":
		until = time.Now().Add(time.Hour).UTC().Format(time.DateTime)
	case "off":
		until = store.Away
	default:
		http.Error(w, "Pick on, pause or off.", http.StatusBadRequest)
		return
	}
	if err := s.Store.SetAwayUntil(u.AccountID, until); err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Availability(until))
}

// logCall saves a finished call to the owner's call history.
func (s *Server) logCall(line string, c calls.Record) {
	err := s.Store.LogCall(line, store.CallRecord{CallID: c.ID, Option: c.Option, Outcome: c.Outcome,
		Started: c.Started, Answered: c.Answered, Ended: c.Ended})
	if err != nil {
		log.Printf("call history %s: %v", line, err)
	}
}
