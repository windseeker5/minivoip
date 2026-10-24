package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"minivoip/calls"
	"minivoip/pages"
	"minivoip/store"
)

// uploadVoicemail takes a caller's recording. No login: the one-time token
// from the call socket says which line it's for.
func (s *Server) uploadVoicemail(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxVoicemailBytes+1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "The message is too long.", http.StatusRequestEntityTooLarge)
		return
	}
	pass, ok := s.Hub.UseVoicemailToken(r.FormValue("token"))
	if !ok {
		http.Error(w, "This voicemail link has expired. Please call again.", http.StatusForbidden)
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "No recording received.", http.StatusBadRequest)
		return
	}
	defer file.Close()
	ct := strings.TrimSpace(strings.Split(header.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(ct, "audio/") && ct != "video/webm" {
		http.Error(w, "That isn't a sound recording.", http.StatusUnsupportedMediaType)
		return
	}
	if ct == "video/webm" {
		ct = "audio/webm"
	}
	ms, _ := strconv.ParseInt(r.FormValue("duration_ms"), 10, 64)
	if _, err := s.Store.AddVoicemail(pass.Line, pass.Option, pass.CallID, ct, ms, file); err != nil {
		serverError(w, r, err)
		return
	}
	s.Hub.NotifyOwners(pass.Line, calls.Message{Type: "voicemail"})
	if s.OnVoicemail != nil {
		s.OnVoicemail(pass.Line)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) voicemailPage(w http.ResponseWriter, r *http.Request, u store.User) {
	list, err := s.Store.Voicemails(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.VoicemailPage(u.Email, list))
}

// voicemailFragment is the phone app's voicemail list alone, to refresh it.
func (s *Server) voicemailFragment(w http.ResponseWriter, r *http.Request, u store.User) {
	list, err := s.Store.Voicemails(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.AppVoicemails(list))
}

func voicemailID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func (s *Server) voicemailAudio(w http.ResponseWriter, r *http.Request, u store.User) {
	path, ct, err := s.Store.VoicemailFile(u.AccountID, voicemailID(r))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

func (s *Server) voicemailListened(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := s.Store.MarkListened(u.AccountID, voicemailID(r)); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) voicemailDelete(w http.ResponseWriter, r *http.Request, u store.User) {
	err := s.Store.DeleteVoicemail(u.AccountID, voicemailID(r))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, safeNext(r.FormValue("back")), http.StatusSeeOther)
}
