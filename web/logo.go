package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"minivoip/store"
)

// logoFile reads the "logo" file from a sign-up or dashboard form: nil when
// none was chosen, store.ErrBadLogo when it can't be a logo.
func logoFile(r *http.Request) ([]byte, error) {
	f, _, err := r.FormFile("logo")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
		return nil, nil
	}
	if err != nil {
		return nil, store.ErrBadLogo // too big for the form, or broken
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, store.MaxLogoBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	return b, store.CheckLogo(b)
}

// uploadLogo sets the line's logo from the dashboard.
func (s *Server) uploadLogo(w http.ResponseWriter, r *http.Request, u store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxLogoBytes+1<<20)
	b, err := logoFile(r)
	switch {
	case errors.Is(err, store.ErrBadLogo) || err == nil && b == nil:
		http.Redirect(w, r, "/dashboard?logo=bad#logo", http.StatusSeeOther)
		return
	case err != nil:
		serverError(w, r, err)
		return
	}
	if err := s.Store.SetLogo(u.AccountID, bytes.NewReader(b)); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard?logo=saved#logo", http.StatusSeeOther)
}

// removeLogo takes the logo off: callers see the initials again.
func (s *Server) removeLogo(w http.ResponseWriter, r *http.Request, u store.User) {
	if err := s.Store.RemoveLogo(u.AccountID); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard?logo=removed#logo", http.StatusSeeOther)
}

// logo serves a business's logo to callers' browsers.
func (s *Server) logo(w http.ResponseWriter, r *http.Request) {
	path := s.Store.LogoPath(r.PathValue("name"))
	if path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}
