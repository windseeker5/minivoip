package web

import (
	"net/http"
	"strings"
)

// askPremium takes the landing page's "Get Premium" email. There are no
// payments yet: Ken sees the list on the admin page and sets people up.
// landing.js sends it and reads the answer; without scripts the visitor
// lands back on the plans.
func (s *Server) askPremium(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	ip, local := clientIP(r)
	switch {
	case !s.sameSite(r):
		http.Error(w, "This form came from another site.", http.StatusForbidden)
		return
	case !local && s.Limits.Premium.Full(ip):
		http.Error(w, "Too many tries from your connection. Please try again in an hour.", http.StatusTooManyRequests)
		return
	case !strings.Contains(email, "@") || len(email) > 200:
		http.Error(w, "Enter your email address.", http.StatusUnprocessableEntity)
		return
	}
	if err := s.Store.AskPremium(email); err != nil {
		serverError(w, r, err)
		return
	}
	if !local {
		s.Limits.Premium.Add(ip)
	}
	http.Redirect(w, r, "/#pricing", http.StatusSeeOther)
}
