// Package web holds the site's routes: the landing page, sign up and log in,
// the owner's dashboard and menu editor, the call pages and the call socket.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"

	"minivoip/calls"
	"minivoip/pages"
	"minivoip/push"
	"minivoip/relay"
	"minivoip/store"
)

const sessionCookie = "mv_session"

type Server struct {
	Store *store.Store
	Hub   *calls.Hub
	// Dev relaxes the same-site check for the dev proxy on localhost:7331.
	Dev bool
	// OnVoicemail runs after a voicemail is saved (push notifications).
	OnVoicemail func(line string)
	// Push sends notifications to the owner's devices (nil in tests).
	Push *push.Sender
	// Admins are emails that can manage promo codes (besides users marked
	// is_admin in the database).
	Admins []string
	// CallLine is the line behind the landing page's "Call us now" button.
	CallLine string
	// Relay is the built-in TURN relay; empty when it isn't running.
	Relay RelayInfo
	// Rooms are the lines' meeting rooms (/room/<line>).
	Rooms *calls.Rooms
}

// RelayInfo tells browsers how to reach the relay.
type RelayInfo struct {
	Secret string
	Host   string
	Port   int
}

type ctxKey struct{}

// Routes registers every page on mux.
func (s *Server) Routes(mux *http.ServeMux) {
	s.Hub.CanAnswer = s.canAnswer
	s.Hub.Available = s.Store.Available
	s.Hub.OnEnd = s.logCall
	s.Rooms.IsOwner = s.canAnswer
	// The owner's open apps show who waits in the meeting room.
	s.Rooms.OnWaiting = func(line string, names []string) {
		s.Hub.NotifyOwners(line, calls.Message{Type: "room", Names: names})
	}

	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /signup", s.signupPage)
	mux.HandleFunc("POST /signup", s.signup)
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /logout", s.logout)

	mux.Handle("GET /dashboard", s.auth(s.dashboard))
	mux.Handle("POST /dashboard/line", s.auth(s.createLine))
	mux.Handle("GET /dashboard/menu", s.auth(s.menuPage))
	mux.Handle("POST /dashboard/menu", s.auth(s.saveMenu))
	mux.Handle("GET /app", s.auth(s.phoneApp))
	mux.Handle("GET /app/voicemails", s.auth(s.voicemailFragment))
	mux.Handle("GET /app/calls", s.auth(s.callsFragment))
	mux.Handle("POST /app/available", s.auth(s.setAvailable))
	mux.Handle("POST /dashboard/prompt", s.auth(s.uploadPrompt))
	mux.Handle("POST /dashboard/logo", s.auth(s.uploadLogo))
	mux.Handle("POST /dashboard/logo/remove", s.auth(s.removeLogo))
	mux.Handle("POST /app/push/subscribe", s.auth(s.pushSubscribe))
	mux.Handle("POST /app/push/unsubscribe", s.auth(s.pushUnsubscribe))
	mux.Handle("POST /app/push/test", s.auth(s.pushTest))
	mux.Handle("POST /app/push/receipt", s.auth(s.pushReceipt))
	mux.Handle("POST /app/calls/{id}/decline", s.auth(s.declineCall))
	mux.HandleFunc("GET /api/android/config", s.androidConfig)
	mux.HandleFunc("GET /download/minivoip.apk", s.androidAPK)

	mux.Handle("GET /admin/promo", s.admin(s.promoPage))
	mux.Handle("POST /admin/promo", s.admin(s.createPromo))
	mux.Handle("POST /admin/promo/{code}/revoke", s.admin(s.revokePromo))
	mux.Handle("POST /admin/accounts/{id}/plan", s.admin(s.setPlan))
	mux.HandleFunc("GET /prompt/{name}", s.prompt)
	mux.HandleFunc("GET /logo/{name}", s.logo)

	mux.Handle("GET /dashboard/voicemail", s.auth(s.voicemailPage))
	mux.Handle("GET /voicemail/{id}/audio", s.auth(s.voicemailAudio))
	mux.Handle("POST /voicemail/{id}/listened", s.auth(s.voicemailListened))
	mux.Handle("POST /voicemail/{id}/delete", s.auth(s.voicemailDelete))
	mux.HandleFunc("POST /api/voicemail", s.uploadVoicemail)

	mux.HandleFunc("GET /call/{line}", s.callPage)
	mux.HandleFunc("GET /api/status/{line}", s.status)
	mux.HandleFunc("GET /room/{line}", s.roomPage)
	mux.Handle("GET /ws/room", s.Rooms)
	mux.HandleFunc("GET /api/ice", s.iceServers)
	mux.Handle("GET /ws", s.Hub)
}

// ------------------------------------------------------------- login state

// user returns the logged-in user, if any.
func (s *Server) user(r *http.Request) (store.User, bool) {
	if u, ok := r.Context().Value(ctxKey{}).(store.User); ok {
		return u, true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return store.User{}, false
	}
	u, err := s.Store.UserBySession(c.Value)
	u.IsAdmin = s.isAdmin(u)
	return u, err == nil
}

// auth sends visitors who aren't logged in to the login page.
func (s *Server) auth(next func(http.ResponseWriter, *http.Request, store.User)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.user(r)
		if !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !s.sameSite(r) {
			http.Error(w, "This form came from another site.", http.StatusForbidden)
			return
		}
		ctx := pages.WithAdmin(context.WithValue(r.Context(), ctxKey{}, u), u.IsAdmin)
		next(w, r.WithContext(ctx), u)
	})
}

// sameSite refuses form posts sent from another website.
func (s *Server) sameSite(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Host == r.Host {
		return true
	}
	return s.Dev && (strings.HasPrefix(u.Host, "localhost:") || strings.HasPrefix(u.Host, "127.0.0.1:"))
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u store.User) error {
	token, expires, err := s.Store.NewSession(u.ID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure(r),
	})
	return nil
}

// secure is true behind HTTPS (directly or through Caddy).
func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// canAnswer lets only the line's owner be its phone app.
func (s *Server) canAnswer(r *http.Request, line string) bool {
	u, ok := s.user(r)
	if !ok {
		return false
	}
	l, err := s.Store.LineForAccount(u.AccountID)
	return err == nil && l.Name == line
}

// safeNext keeps ?next= redirects on this site.
func safeNext(next string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/dashboard"
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render %s: %v", r.URL.Path, err)
	}
}

func serverError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}

// ------------------------------------------------------------- pages

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := s.user(r)
	// minivoip's own line: the hero plays its greeting, "Call us now" calls it.
	l, err := s.Store.LineByName(s.CallLine)
	if s.CallLine == "" || err != nil {
		if s.CallLine != "" && !errors.Is(err, store.ErrNotFound) {
			log.Printf("home: line %s: %v", s.CallLine, err)
		}
		render(w, r, pages.Home(loggedIn, nil, baseURL(r)))
		return
	}
	render(w, r, pages.Home(loggedIn, &l, baseURL(r)))
}

// ------------------------------------------------------------- sign up, log in

func (s *Server) signupPage(w http.ResponseWriter, r *http.Request) {
	// Already logged in: say so, instead of a silent jump to the dashboard.
	if u, ok := s.user(r); ok {
		render(w, r, pages.SignupLoggedIn(u.Email))
		return
	}
	render(w, r, pages.Signup(pages.AccountForm{}))
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	f := pages.AccountForm{Email: strings.TrimSpace(r.FormValue("email")), Promo: strings.TrimSpace(r.FormValue("promo"))}
	password := r.FormValue("password")
	switch {
	case !s.sameSite(r):
		f.Error = "This form came from another site."
	case !strings.Contains(f.Email, "@"):
		f.Error = "Enter your email address."
	case len(password) < 8:
		f.Error = "Use a password of at least 8 characters."
	}
	if f.Error == "" {
		u, err := s.Store.CreateAccount(f.Email, password, f.Promo)
		switch {
		case errors.Is(err, store.ErrEmailTaken):
			f.Error = "There's already an account with this email. Log in instead."
		case errors.Is(err, store.ErrBadPromo):
			f.Error = "This promo code isn't valid. Check it, or leave the box empty."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			if err := s.startSession(w, r, u); err != nil {
				serverError(w, r, err)
				return
			}
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(w, r, pages.Signup(f))
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(r); ok {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	f := pages.AccountForm{Next: r.URL.Query().Get("next")}
	if strings.HasPrefix(f.Next, "/app") {
		render(w, r, pages.AppLogin(f))
		return
	}
	render(w, r, pages.Login(f))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	f := pages.AccountForm{Email: strings.TrimSpace(r.FormValue("email")), Next: r.FormValue("next")}
	if !s.sameSite(r) {
		f.Error = "This form came from another site."
	} else {
		u, err := s.Store.Login(f.Email, r.FormValue("password"))
		switch {
		case errors.Is(err, store.ErrBadLogin):
			f.Error = "Wrong email or password."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			if err := s.startSession(w, r, u); err != nil {
				serverError(w, r, err)
				return
			}
			http.Redirect(w, r, safeNext(f.Next), http.StatusSeeOther)
			return
		}
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	if strings.HasPrefix(f.Next, "/app") {
		render(w, r, pages.AppLogin(f))
		return
	}
	render(w, r, pages.Login(f))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && s.sameSite(r) {
		s.Store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1})
	// The phone app logs out to its own login screen.
	switch r.FormValue("next") {
	case "/app":
		http.Redirect(w, r, "/login?next=/app", http.StatusSeeOther)
		return
	case "/signup": // "Log out and sign up" on the sign up page
		http.Redirect(w, r, "/signup", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// iceServers gives a browser its connection helpers for one call: STUN and
// a two-hour relay login. Without the relay, a public STUN server.
func (s *Server) iceServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if s.Relay.Secret == "" {
		w.Write([]byte(`{"iceServers":[{"urls":["stun:stun.l.google.com:19302"]}]}`))
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"iceServers": relay.ICEServers(s.Relay.Secret, s.Relay.Host, s.Relay.Port, "call")})
}
