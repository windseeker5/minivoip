package web

import (
	"errors"
	"net/http"

	"minivoip/pages"
	"minivoip/store"
)

// onPremium says whether the account has Premium right now.
func (s *Server) onPremium(u store.User) bool {
	b, err := s.Store.Billing(u.AccountID)
	return err == nil && b.Plan == store.Premium
}

// notesPage is the Notes tab: Premium's notes, or what they'd be on Basic.
func (s *Server) notesPage(w http.ResponseWriter, r *http.Request, u store.User) {
	list, err := s.Store.Notes(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !s.onPremium(u) {
		l, err := s.Store.Language(u.AccountID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		render(w, r, pages.NotesLocked(u.Email, l, len(list)))
		return
	}
	d := pages.NotesTab{Email: u.Email, List: list}
	if d.Seconds, err = s.Store.NoteSecondsThisMonth(u.AccountID); err == nil {
		if d.Language, err = s.Store.Language(u.AccountID); err == nil {
			d.TakeNotes, err = s.Store.TakeNotes(u.AccountID)
		}
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Notes(d))
}

// notePage is one note, for its own account on Premium only.
func (s *Server) notePage(w http.ResponseWriter, r *http.Request, u store.User) {
	if !s.onPremium(u) {
		http.NotFound(w, r)
		return
	}
	n, err := s.Store.NoteByID(u.AccountID, pathID(r))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	l, err := s.Store.Language(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.NotePage(u.Email, l, n))
}

// noteDelete works on any plan: what's said about someone can always go.
func (s *Server) noteDelete(w http.ResponseWriter, r *http.Request, u store.User) {
	err := s.Store.DeleteNote(u.AccountID, pathID(r))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/notes", http.StatusSeeOther)
}

// notesSwitch is the "Take notes" switch: a ticked box sends on, an unticked one
// sends nothing.
func (s *Server) notesSwitch(w http.ResponseWriter, r *http.Request, u store.User) {
	if !s.onPremium(u) {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	if err := s.Store.SetTakeNotes(u.AccountID, r.FormValue("on") == "1"); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/notes", http.StatusSeeOther)
}
