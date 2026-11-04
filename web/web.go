// Package web holds the site's routes: the landing page, sign up and log in,
// the owner's dashboard and menu editor, the call pages and the call socket.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
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

const (
	sessionCookie = "mv_session"
	knownCookie   = "mv_known"
)

type Server struct {
	Store *store.Store
	Hub   *calls.Hub
	// Dev relaxes the same-site check for the dev proxy on localhost:7331.
	Dev bool
	// OnVoicemail runs after a voicemail is saved (push notifications).
	OnVoicemail func(line string)
	// OnNote runs when a note is queued, to wake the notes worker.
	OnNote func()
	// Push sends notifications to the owner's devices (nil in tests).
	Push *push.Sender
	// CallLine is the line behind the landing page's "Call us now" button.
	CallLine string
	// Relay is the built-in TURN relay; empty when it isn't running.
	Relay RelayInfo
	// Rooms are the lines' meeting rooms (/room/<line>).
	Rooms *calls.Rooms
	// Limits count calls, messages, log ins and sign ups (limits.go); Routes
	// sets the agreed ones when nil.
	Limits *Limits
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
	if s.Limits == nil {
		s.Limits = NewLimits()
	}
	s.Hub.CanAnswer = s.canAnswer
	s.Hub.AllowCall = s.allowCall
	s.Hub.AllowWake = s.allowWake
	s.Hub.AllowVoicemail = s.allowVoicemail
	s.Hub.ICE = s.relayLogin
	s.Rooms.ICE = s.relayLogin
	s.Rooms.AllowKnock = s.allowKnock
	s.Rooms.GuestKey = func(r *http.Request) string {
		if ip, local := clientIP(r); !local {
			return ip
		}
		return ""
	}
	s.Hub.CleanOption = s.cleanOption
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
	mux.Handle("GET /dashboard/password", s.auth(s.passwordPage))
	mux.Handle("POST /dashboard/password", s.auth(s.changePassword))
	mux.Handle("GET /dashboard/language", s.auth(s.languagePage))
	mux.Handle("POST /dashboard/language", s.auth(s.saveLanguage))
	mux.HandleFunc("POST /premium", s.askPremium)

	mux.Handle("GET /dashboard", s.auth(s.dashboard))
	mux.Handle("POST /dashboard/line", s.auth(s.createLine))
	mux.Handle("GET /dashboard/menu", s.auth(s.menuPage))
	mux.Handle("GET /dashboard/business", s.auth(s.businessPage))
	mux.Handle("POST /dashboard/menu", s.auth(s.saveMenu))
	mux.Handle("POST /dashboard/business", s.auth(s.saveBusiness))
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
	mux.Handle("GET /dashboard/notes", s.auth(s.notesPage))
	mux.Handle("POST /dashboard/notes/switch", s.auth(s.notesSwitch))
	mux.Handle("GET /dashboard/notes/{id}", s.auth(s.notePage))
	mux.Handle("POST /dashboard/notes/{id}/delete", s.auth(s.noteDelete))

	mux.Handle("GET /voicemail/{id}/audio", s.auth(s.voicemailAudio))
	mux.Handle("POST /voicemail/{id}/listened", s.auth(s.voicemailListened))
	mux.Handle("POST /voicemail/{id}/delete", s.auth(s.voicemailDelete))
	mux.HandleFunc("POST /api/voicemail", s.uploadVoicemail)

	mux.HandleFunc("GET /call/{line}", s.callPage)
	mux.HandleFunc("GET /api/status/{line}", s.status)
	mux.HandleFunc("GET /room/{line}", s.roomPage)
	mux.Handle("GET /ws/room", s.socket(s.Rooms))
	mux.HandleFunc("GET /api/ice", s.iceServers)
	mux.Handle("GET /ws", s.socket(s.Hub))
}

// ------------------------------------------------------------- login state

// user returns the logged-in user, if any.
func (s *Server) user(r *http.Request) (store.User, bool) {
	if u, ok := r.Context().Value(ctxKey{}).(store.User); ok {
		return u, true
	}
	token, ok := readCookie(r, sessionCookie)
	if !ok {
		return store.User{}, false
	}
	u, err := s.Store.UserBySession(token)
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
		ctx := pages.WithViewer(context.WithValue(r.Context(), ctxKey{}, u), pages.Viewer{
			Admin:       u.IsAdmin,
			NewMessages: func() int { return s.Store.NewVoicemails(u.AccountID) },
			Premium:     func() bool { return s.onPremium(u) },
		})
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
	setCookie(w, r, sessionCookie, token, expires)
	s.markKnown(w, r, u.Email)
	return nil
}

// setCookie writes a cookie. Behind HTTPS its name starts with __Host-, which
// tells the browser to refuse one set by another dresdell.com site.
func setCookie(w http.ResponseWriter, r *http.Request, name, value string, expires time.Time) {
	if secure(r) {
		name = "__Host-" + name
	}
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure(r),
	})
}

// readCookie reads what setCookie wrote. Behind HTTPS the plain name is never
// believed: another dresdell.com site could have set it.
func readCookie(r *http.Request, name string) (string, bool) {
	if secure(r) {
		name = "__Host-" + name
	}
	c, err := r.Cookie(name)
	if err != nil {
		return "", false
	}
	return c.Value, true
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

// safeNext keeps ?next= redirects on this site. Browsers read a backslash as a
// slash and skip tabs and new lines, so those can hide another site's address.
func safeNext(next string) string {
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsFunc(next, func(r rune) bool { return r == '\\' || r < ' ' || r == 0x7f }) {
		return "/dashboard"
	}
	return next
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
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	f := pages.AccountForm{Email: strings.TrimSpace(r.FormValue("email")), Promo: strings.TrimSpace(r.FormValue("promo")), Language: store.Language(r.FormValue("language"))}
	if !f.Language.Valid() {
		f.Language = store.French
	}
	password := r.FormValue("password")
	if !s.sameSite(r) {
		f.Error = "This form came from another site."
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, pages.Signup(f))
		return
	}
	if !s.takeSignup(r) {
		f.Error = "Too many tries from your connection. Please try again in an hour."
		w.WriteHeader(http.StatusTooManyRequests)
		render(w, r, pages.Signup(f))
		return
	}
	switch {
	case !validEmail(f.Email):
		f.Error = "Enter your email address."
	case len(password) < 8 || len(password) > store.MaxPassword:
		f.Error = passwordRule
	case f.Promo == "":
		f.Error = "Enter your promo code. You need one to get a line for now."
	}
	if f.Error == "" {
		u, err := s.Store.CreateAccount(f.Email, password, f.Promo)
		switch {
		case errors.Is(err, store.ErrEmailTaken):
			f.Error = "There's already an account with this email. Log in instead."
		case errors.Is(err, store.ErrBadPromo):
			f.Error = "This promo code isn't valid. Check it and try again."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			s.signupDone(r, true)
			// A failure here leaves a working French account, changed from the account menu.
			if err := s.Store.SetLanguage(u.AccountID, f.Language); err != nil {
				log.Printf("signup: language: %v", err)
			}
			if err := s.startSession(w, r, u); err != nil {
				serverError(w, r, err)
				return
			}
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
	}
	s.signupDone(r, false)
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
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	f := pages.AccountForm{Email: strings.TrimSpace(r.FormValue("email")), Next: r.FormValue("next")}
	status := http.StatusUnprocessableEntity
	if !s.sameSite(r) {
		f.Error = "This form came from another site."
	} else if !s.takeLogin(r, f.Email) {
		f.Error = "Too many wrong passwords. Please wait 15 minutes, then try again."
		status = http.StatusTooManyRequests
	} else {
		u, err := s.Store.Login(f.Email, r.FormValue("password"))
		switch {
		case errors.Is(err, store.ErrBadLogin):
			f.Error = "Wrong email or password."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			s.loginWorked(r, f.Email)
			if err := s.startSession(w, r, u); err != nil {
				serverError(w, r, err)
				return
			}
			http.Redirect(w, r, safeNext(f.Next), http.StatusSeeOther)
			return
		}
	}
	w.WriteHeader(status)
	if strings.HasPrefix(f.Next, "/app") {
		render(w, r, pages.AppLogin(f))
		return
	}
	render(w, r, pages.Login(f))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	if !s.sameSite(r) {
		http.Error(w, "This form came from another site.", http.StatusForbidden)
		return
	}
	u, loggedIn := s.user(r)
	if token, ok := readCookie(r, sessionCookie); ok {
		s.Store.DeleteSession(token)
	}
	if loggedIn {
		s.loginsChanged(u)
	}
	setCookie(w, r, sessionCookie, "", time.Unix(0, 0))
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

// loginsChanged closes the account's sockets whose login ended, so a phone that
// logged out doesn't keep ringing or hosting.
func (s *Server) loginsChanged(u store.User) {
	if l, err := s.Store.LineForAccount(u.AccountID); err == nil {
		s.Hub.Recheck(l.Name)
		s.Rooms.Recheck(l.Name)
	}
}

// passwordRule is what sign up and "Change password" ask for.
const passwordRule = "Use a password of 8 to 72 characters."

// validEmail is a plain address that fits: nothing that could hide extra parts
// in a mailto: link on the admin page.
func validEmail(email string) bool {
	if len(email) > 254 || strings.ContainsAny(email, "?&=#%<>\"") {
		return false
	}
	// Plain letters only: some other alphabets' letters look like, or lower to,
	// plain ones, and would make a look-alike of someone's account.
	if strings.IndexFunc(email, func(c rune) bool { return c > 126 }) >= 0 {
		return false
	}
	a, err := mail.ParseAddress(email)
	return err == nil && a.Address == email
}

func (s *Server) passwordPage(w http.ResponseWriter, r *http.Request, u store.User) {
	render(w, r, pages.Password(u.Email, "", r.URL.Query().Get("saved") == "1"))
}

// changePassword also logs out the account's other devices.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	next := r.FormValue("new")
	msg := ""
	switch {
	case len(next) < 8 || len(next) > store.MaxPassword:
		msg = passwordRule
	case !s.takeLogin(r, u.Email):
		msg = "Too many wrong passwords. Please wait 15 minutes, then try again."
	default:
		token, ok := readCookie(r, sessionCookie)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		switch err := s.Store.ChangePassword(u.ID, r.FormValue("current"), next, token); {
		case errors.Is(err, store.ErrBadLogin):
			msg = "Your current password isn't right."
		case err != nil:
			serverError(w, r, err)
			return
		default:
			s.loginWorked(r, u.Email)
			s.markKnown(w, r, u.Email)
			s.loginsChanged(u)
			http.Redirect(w, r, "/dashboard/password?saved=1", http.StatusSeeOther)
			return
		}
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	render(w, r, pages.Password(u.Email, msg, false))
}

func (s *Server) languagePage(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.Language(u.AccountID)
	if err != nil {
		serverError(w, r, err)
		return
	}
	render(w, r, pages.Language(u.Email, l, "", r.URL.Query().Get("saved") == "1"))
}

func (s *Server) saveLanguage(w http.ResponseWriter, r *http.Request, u store.User) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	l := store.Language(r.FormValue("language"))
	if !l.Valid() {
		current, err := s.Store.Language(u.AccountID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		render(w, r, pages.Language(u.Email, current, "Choose French or English.", false))
		return
	}
	if err := s.Store.SetLanguage(u.AccountID, l); err != nil {
		serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/dashboard/language?saved=1", http.StatusSeeOther)
}

// relayLogin is the connection helpers with a two-hour relay login. The relay
// isn't free for all: a caller gets this only once the owner answers
// ("accepted"), a guest once let in ("welcome"), and the owner by being logged
// in (iceServers).
func (s *Server) relayLogin() json.RawMessage {
	if s.Relay.Secret == "" {
		return s.stunOnly()
	}
	b, _ := json.Marshal(map[string]any{"iceServers": relay.ICEServers(s.Relay.Secret, s.Relay.Host, s.Relay.Port, "call")})
	return b
}

// stunOnly lets a browser find its own address, with no way to use the relay.
func (s *Server) stunOnly() json.RawMessage {
	if s.Relay.Secret == "" {
		return json.RawMessage(`{"iceServers":[{"urls":["stun:stun.l.google.com:19302"]}]}`)
	}
	b, _ := json.Marshal(map[string]any{"iceServers": relay.STUNServer(s.Relay.Host, s.Relay.Port)})
	return b
}

// iceServers is what a browser asks before a call: a relay login for a
// logged-in owner, 30 in 10 minutes, and otherwise only the address helper.
func (s *Server) iceServers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	ip, local := clientIP(r)
	if _, loggedIn := s.user(r); !loggedIn || !local && !s.Limits.Relay.Take(ip) {
		w.Write(s.stunOnly())
		return
	}
	w.Write(s.relayLogin())
}
