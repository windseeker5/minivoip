package web

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"minivoip/calls"
	"minivoip/store"
)

func testSite(t *testing.T) (*Server, *store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := &Server{Store: st, Hub: calls.NewHub(st.LineExists), Rooms: calls.NewRooms(st.HasRoom)}
	mux := http.NewServeMux()
	s.Routes(mux)
	return s, st, s.Secured(mux)
}

// post sends a form as a visitor from a public address, through Caddy.
func post(h http.Handler, path string, form url.Values, cookie string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if cookie != "" {
		r.Header.Set("Cookie", sessionCookie+"="+cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSafeNext(t *testing.T) {
	for next, want := range map[string]string{
		"/app":                 "/app",
		"/app?answer=1":        "/app?answer=1",
		"/dashboard/menu#x":    "/dashboard/menu#x",
		"":                     "/dashboard",
		"//evil.example":       "/dashboard",
		"/\\evil.example/x":    "/dashboard",
		"/\t/evil.example":     "/dashboard",
		"/\n/evil.example":     "/dashboard",
		"https://evil.example": "/dashboard",
		"javascript:alert(1)":  "/dashboard",
		"evil.example":         "/dashboard",
	} {
		if got := safeNext(next); got != want {
			t.Errorf("%q: got %q", next, got)
		}
	}
}

func TestSignupNeedsACode(t *testing.T) {
	_, st, h := testSite(t)
	form := url.Values{"email": {"a@example.com"}, "password": {"long-enough"}}
	if w := post(h, "/signup", form, ""); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Enter your promo code") {
		t.Fatalf("no code: %d", w.Code)
	}
	if _, err := st.Login("a@example.com", "long-enough"); err == nil {
		t.Fatal("an account was made without a code")
	}
	if err := st.CreatePromoCode("FRIENDS-2027", store.Basic, 0, "", ""); err != nil {
		t.Fatal(err)
	}
	form.Set("promo", "friends-2027")
	if w := post(h, "/signup", form, ""); w.Code != http.StatusSeeOther {
		t.Fatalf("with a code: %d", w.Code)
	}
}

// Sign up keeps the language chosen, French unless English was picked, and a
// made-up one is French too.
func TestSignupLanguage(t *testing.T) {
	for sent, want := range map[string]store.Language{"": store.French, "fr": store.French, "en": store.English, "de": store.French} {
		_, st, h := testSite(t) // each its own site: sign ups per address are limited
		st.CreatePromoCode("FRIENDS-2027", store.Basic, 0, "", "")
		form := url.Values{"email": {"joe@example.com"}, "password": {"long-enough"}, "promo": {"FRIENDS-2027"}, "language": {sent}}
		if w := post(h, "/signup", form, ""); w.Code != http.StatusSeeOther {
			t.Fatalf("%q: %d", sent, w.Code)
		}
		u, err := st.Login("joe@example.com", "long-enough")
		if err != nil {
			t.Fatal(err)
		}
		if l, _ := st.Language(u.AccountID); l != want {
			t.Errorf("%q: got %q", sent, l)
		}
	}
	// The form comes back with the choice kept when something else is wrong.
	_, st, h := testSite(t)
	st.CreatePromoCode("FRIENDS-2027", store.Basic, 0, "", "")
	w := post(h, "/signup", url.Values{"email": {"a@example.com"}, "password": {"short"}, "promo": {"FRIENDS-2027"}, "language": {"en"}}, "")
	if !regexp.MustCompile(`value="en"\s+checked`).MatchString(w.Body.String()) {
		t.Fatal("the choice of English was lost")
	}
}

func TestChangeLanguage(t *testing.T) {
	_, st, h := testSite(t)
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	other, _ := st.CreateAccount("sam@example.com", "long-enough", "")
	token, _, _ := st.NewSession(u.ID)
	if w := post(h, "/dashboard/language", url.Values{"language": {"en"}}, ""); w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Fatalf("logged out: %d %s", w.Code, w.Header().Get("Location"))
	}
	if l, _ := st.Language(u.AccountID); l != store.French {
		t.Fatalf("changed while logged out: %q", l)
	}
	if w := post(h, "/dashboard/language", url.Values{"language": {"de"}}, token); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a made-up language: %d", w.Code)
	}
	if l, _ := st.Language(u.AccountID); l != store.French {
		t.Fatalf("a made-up language changed it: %q", l)
	}
	if w := post(h, "/dashboard/language", url.Values{"language": {"en"}}, token); w.Code != http.StatusSeeOther {
		t.Fatalf("saving: %d", w.Code)
	}
	if l, _ := st.Language(u.AccountID); l != store.English {
		t.Fatalf("saved: %q", l)
	}
	if l, _ := st.Language(other.AccountID); l != store.French {
		t.Fatalf("someone else's account changed: %q", l)
	}
	// From another website, the same post is refused.
	r := httptest.NewRequest("POST", "/dashboard/language", strings.NewReader("language=fr"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Cookie", sessionCookie+"="+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("from another site: %d", rec.Code)
	}
	if l, _ := st.Language(u.AccountID); l != store.English {
		t.Fatalf("another site changed it: %q", l)
	}
}

// Guessing codes or emails stops after 10 refused tries, even though each one
// is refused for its own reason.
func TestSignupGuessingStops(t *testing.T) {
	_, _, h := testSite(t)
	for i := 0; i < 10; i++ {
		form := url.Values{"email": {"a@example.com"}, "password": {"long-enough"}, "promo": {"GUESS-1234"}}
		if w := post(h, "/signup", form, ""); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("try %d: %d", i+1, w.Code)
		}
	}
	form := url.Values{"email": {"a@example.com"}, "password": {"long-enough"}, "promo": {"GUESS-1234"}}
	if w := post(h, "/signup", form, ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th try: %d", w.Code)
	}
}

func TestPagesGetSecurityHeaders(t *testing.T) {
	_, _, h := testSite(t)
	get := func(path string, https bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if https {
			r.Header.Set("X-Forwarded-Proto", "https")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	home := get("/login", true)
	if home.Header().Get("X-Content-Type-Options") != "nosniff" || home.Header().Get("X-Frame-Options") != "DENY" ||
		!strings.Contains(home.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		home.Header().Get("Strict-Transport-Security") == "" {
		t.Fatalf("login headers: %v", home.Header())
	}
	if plain := get("/login", false); plain.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS without HTTPS")
	}
	call := get("/call/nobody", true)
	if call.Header().Get("X-Frame-Options") != "SAMEORIGIN" || !strings.Contains(call.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatalf("the call window sits in our own pages only: %v", call.Header())
	}
	if strings.Contains(home.Header().Get("Content-Security-Policy"), "google") {
		t.Fatalf("fonts come from this site: %v", home.Header())
	}
	if strings.Contains(home.Header().Get("Content-Security-Policy"), "script-src 'self' ") || strings.Contains(home.Header().Get("Content-Security-Policy"), "unsafe-eval") {
		t.Fatalf("scripts come from this site, nothing else: %v", home.Header())
	}
}

func TestAudioTypes(t *testing.T) {
	for in, want := range map[string]string{
		"audio/webm;codecs=opus": "audio/webm",
		"video/webm":             "audio/webm",
		"AUDIO/OGG":              "audio/ogg",
		"audio/mp4":              "audio/mp4",
		"audio/mpeg":             "audio/mpeg",
		"audio/x,text/html":      "",
		"audio/x":                "",
		"text/html":              "",
		"audio/":                 "",
		"":                       "",
	} {
		if got := store.AudioType(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}

func TestVoicemailCantBeAWebPage(t *testing.T) {
	_, st, h := testSite(t)
	u, err := st.CreateAccount("joe@example.com", "long-enough", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateLine(u.AccountID, "joesbnb", "Joe's"); err != nil {
		t.Fatal(err)
	}
	// A row from before uploads checked their type still plays as a recording.
	vm, err := st.AddVoicemail("joesbnb", "", "", "audio/x,text/html", 1000, strings.NewReader("<script>alert(1)</script>"))
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := st.NewSession(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/voicemail/"+itoa(vm.ID)+"/audio", nil)
	r.Header.Set("Cookie", sessionCookie+"="+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "audio/webm" {
		t.Errorf("served as %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("headers: %v", w.Header())
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// A free-floating push address is refused for a stranger, whatever it says.
func TestPushSubscribeRefusesOtherAddresses(t *testing.T) {
	s, st, h := testSite(t)
	s.Dev = true // the public test site runs in development mode
	u, err := st.CreateAccount("joe@example.com", "long-enough", "")
	if err != nil {
		t.Fatal(err)
	}
	token, _, _ := st.NewSession(u.ID)
	send := func(endpoint, xff string) int {
		body := `{"endpoint":"` + endpoint + `","keys":{"p256dh":"a","auth":"b"}}`
		r := httptest.NewRequest("POST", "/app/push/subscribe", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:5000"
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		r.Header.Set("Cookie", sessionCookie+"="+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, e := range []string{"http://localhost:2019/stop", "http://localhost.evil.example/x", "https://192.168.1.254:8443/x", "https://evil.example/x"} {
		if c := send(e, "198.51.100.7"); c != http.StatusBadRequest {
			t.Errorf("%s from a stranger: %d", e, c)
		}
	}
	if c := send("https://fcm.googleapis.com/fcm/send/x", "198.51.100.7"); c != http.StatusNoContent {
		t.Errorf("a real push service: %d", c)
	}
	if c := send("http://localhost:2019/test", ""); c != http.StatusNoContent {
		t.Errorf("the browser test on this machine: %d", c)
	}
}

// One line per account.
func TestOneLinePerAccount(t *testing.T) {
	_, st, _ := testSite(t)
	u, err := st.CreateAccount("joe@example.com", "long-enough", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateLine(u.AccountID, "first", "Joe's"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateLine(u.AccountID, "second", "Joe's"); err != store.ErrHasLine {
		t.Fatalf("second line: %v", err)
	}
}

// Logging out from another site does nothing.
func TestLogoutFromAnotherSite(t *testing.T) {
	_, st, h := testSite(t)
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	token, _, _ := st.NewSession(u.ID)
	r := httptest.NewRequest("POST", "/logout", nil)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Cookie", sessionCookie+"="+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("%d, cookie %q", w.Code, w.Header().Get("Set-Cookie"))
	}
	if _, err := st.UserBySession(token); err != nil {
		t.Fatal("the login ended")
	}
}

func caller(ip string) *http.Request {
	r := httptest.NewRequest("GET", "/ws", nil)
	r.RemoteAddr = ip + ":5000"
	return r
}

// A visitor calls 3 times in 10 minutes. 20 networks an hour (here 2) wake a
// line's phones; a prankster's repeat calls count once.
func TestCallLimits(t *testing.T) {
	s, _, _ := testSite(t)
	s.Limits.LineCalls = NewWindow(2, s.Limits.LineCalls.Per)
	for i := 0; i < 3; i++ {
		if !s.allowCall(caller("198.51.100.1"), "joe") {
			t.Fatalf("call %d refused", i+1)
		}
		if !s.allowWake(caller("198.51.100.1"), "joe") {
			t.Fatalf("call %d didn't wake the phones", i+1)
		}
	}
	if s.allowCall(caller("198.51.100.1"), "joe") {
		t.Fatal("a 4th call from one visitor")
	}
	if !s.allowWake(caller("198.51.100.2"), "joe") {
		t.Fatal("an honest caller didn't wake the phones")
	}
	// The line is full: one call in 5 minutes still wakes the phones, so it
	// can't be silenced. The others may call, and ring the owner's open apps.
	if !s.allowWake(caller("198.51.100.3"), "joe") {
		t.Fatal("the first call past the budget should wake the phones")
	}
	if !s.allowCall(caller("198.51.100.4"), "joe") || s.allowWake(caller("198.51.100.4"), "joe") {
		t.Fatal("the next network should call without waking the phones")
	}
	if s.allowWake(caller("198.51.100.1"), "joe") {
		t.Fatal("the prankster woke the phones past the budget")
	}
	// One IPv6 customer's /48 counts once.
	s, _, _ = testSite(t)
	s.Limits.LineCalls = NewWindow(2, s.Limits.LineCalls.Per)
	if !s.allowWake(caller("[2001:db8:1:1::1]"), "joe") || !s.allowWake(caller("[2001:db8:1:2::1]"), "joe") || !s.allowWake(caller("[2001:db8:1:3::1]"), "joe") {
		t.Fatal("/64s of one /48 count once")
	}
	if !s.allowWake(caller("[2001:db8:2:1::1]"), "joe") {
		t.Fatal("another /48 is another network")
	}
	if !s.allowWake(caller("[2001:db8:3:1::1]"), "joe") || s.allowWake(caller("[2001:db8:4:1::1]"), "joe") {
		t.Fatal("the budget is 2 networks, then one call in 5 minutes")
	}
	// A visitor has a daily limit too.
	s, _, _ = testSite(t)
	s.Limits.Calls = NewWindow(100, s.Limits.Calls.Per)
	for i := 0; i < 12; i++ {
		if !s.allowCall(caller("198.51.100.1"), "joe") {
			t.Fatalf("call %d", i+1)
		}
	}
	if s.allowCall(caller("198.51.100.1"), "joe") {
		t.Fatal("a 13th call in a day")
	}
}

// burst sends n requests at the same moment and counts the answers.
func burst(n int, send func(i int) int) map[int]int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	got := map[int]int{}
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			c := send(i)
			mu.Lock()
			got[c]++
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()
	return got
}

// Guesses sent at the same moment count too.
func TestLoginBurstIsLimited(t *testing.T) {
	_, st, h := testSite(t)
	st.CreateAccount("joe@example.com", "the-right-password", "")
	got := burst(200, func(int) int {
		return post(h, "/login", url.Values{"email": {"joe@example.com"}, "password": {"wrong-password"}}, "").Code
	})
	if got[http.StatusUnprocessableEntity] != 5 || got[http.StatusTooManyRequests] != 195 {
		t.Fatalf("answers: %v", got)
	}
	if w := post(h, "/login", url.Values{"email": {"joe@example.com"}, "password": {"the-right-password"}}, ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("the right password after the burst: %d", w.Code)
	}
}

// Strangers with many addresses use up an account's wrong passwords, but the
// browser its owner already logged in on still gets in, and only that account's.
func TestKnownBrowserIsNotLockedOut(t *testing.T) {
	s, st, h := testSite(t)
	st.CreateAccount("joe@example.com", "the-right-password", "")
	st.CreateAccount("sue@example.com", "the-right-password", "")
	login := func(addr, email, password, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"email": {email}, "password": {password}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", addr)
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("Origin", "https://example.com")
		r.Host = "example.com"
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := login("203.0.113.200", "joe@example.com", "the-right-password", "")
	var known, session *http.Cookie
	for _, c := range w.Result().Cookies() {
		switch c.Name {
		case "__Host-" + knownCookie:
			known = c
		case "__Host-" + sessionCookie:
			session = c
		}
	}
	if known == nil || session == nil || !known.Secure || !session.Secure || known.Path != "/" || session.Path != "/" || !session.HttpOnly {
		t.Fatalf("cookies behind HTTPS: %v", w.Result().Cookies())
	}
	mark := known.Name + "=" + known.Value
	for i := 0; i < s.Limits.Accounts.Max; i++ {
		login("198.51."+strconv.Itoa(i/250)+"."+strconv.Itoa(i%250+1), "joe@example.com", "wrong-password", "")
	}
	if c := login("203.0.113.201", "joe@example.com", "the-right-password", "").Code; c != http.StatusTooManyRequests {
		t.Fatalf("a new browser while the account is under attack: %d", c)
	}
	if c := login("203.0.113.202", "joe@example.com", "the-right-password", mark).Code; c != http.StatusSeeOther {
		t.Fatalf("the owner's own browser: %d", c)
	}
	for i := 0; i < 5; i++ {
		login("203.0.113.203", "joe@example.com", "wrong-password", mark)
	}
	if c := login("203.0.113.203", "joe@example.com", "the-right-password", mark).Code; c != http.StatusTooManyRequests {
		t.Fatalf("the mark must not lift the limit on one address: %d", c)
	}
	for i := 0; i < s.Limits.Accounts.Max; i++ {
		login("192.0."+strconv.Itoa(i/250)+"."+strconv.Itoa(i%250+1), "sue@example.com", "wrong-password", "")
	}
	if c := login("203.0.113.204", "sue@example.com", "the-right-password", mark).Code; c != http.StatusTooManyRequests {
		t.Fatalf("joe's mark on sue's account: %d", c)
	}
}

// Behind HTTPS only the __Host- name counts: another dresdell.com site can set
// the plain one.
func TestPlainSessionCookieIsRefusedBehindHTTPS(t *testing.T) {
	s, st, _ := testSite(t)
	u, err := st.CreateAccount("joe@example.com", "the-right-password", "")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := st.NewSession(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Cookie", sessionCookie+"="+token)
	if _, ok := s.user(r); ok {
		t.Fatal("a plain cookie logged in behind HTTPS")
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("Cookie", "__Host-"+sessionCookie+"="+token)
	if _, ok := s.user(r); !ok {
		t.Fatal("the __Host- cookie doesn't work")
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", sessionCookie+"="+token)
	if _, ok := s.user(r); !ok {
		t.Fatal("the plain cookie doesn't work on localhost")
	}
}

// A new password ends the browser marks, and a look-alike email can't sign up.
func TestKnownMarkFollowsThePassword(t *testing.T) {
	s, st, _ := testSite(t)
	u, err := st.CreateAccount("joe@example.com", "the-right-password", "")
	if err != nil {
		t.Fatal(err)
	}
	before := s.knownMark("joe@example.com")
	if before == "" || s.knownMark("nobody@example.com") != "" {
		t.Fatal("marks")
	}
	if err := st.ChangePassword(u.ID, "the-right-password", "another-password", ""); err != nil {
		t.Fatal(err)
	}
	if s.knownMark("joe@example.com") == before {
		t.Fatal("the mark survived a new password")
	}
	if validEmail("joe\u212a@example.com") || validEmail("\u0130joe@example.com") {
		t.Fatal("a look-alike email passed")
	}
	if !validEmail("joe@example.com") {
		t.Fatal("a plain email was refused")
	}
}

// One IPv6 /48 holds thousands of /64s: each starts fresh, but the network
// shares a budget (the review's 200 of 200 calls and 120 of 120 messages).
func TestOneIPv6NetworkIsLimited(t *testing.T) {
	s, _, _ := testSite(t)
	from := func(i int) *http.Request {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("2001:db8:1:%x::1", i))
		return r
	}
	count := func(try func(*http.Request) bool) int {
		n := 0
		for i := 0; i < 200; i++ {
			if try(from(i)) {
				n++
			}
		}
		return n
	}
	if n := count(func(r *http.Request) bool { return s.allowCall(r, "joe") }); n != 9 {
		t.Fatalf("calls that rang: %d", n)
	}
	if n := count(func(r *http.Request) bool { return s.takeVoicemail(r, "joe") }); n != 9 {
		t.Fatalf("messages stored: %d", n)
	}
	if n := count(func(r *http.Request) bool { return s.takeLogin(r, "joe@example.com") }); n != 50 {
		t.Fatalf("password tries: %d", n)
	}
	// Refused by a /64's own limit, the network gets its count back.
	w := NewWindow(1, time.Hour).perNetwork(2)
	if !w.Take("2001:db8:1:1::/64 joe") || w.Take("2001:db8:1:1::/64 joe") || !w.Take("2001:db8:1:2::/64 joe") || w.Take("2001:db8:1:3::/64 joe") {
		t.Fatal("the network's count")
	}
	// Another network isn't touched.
	r := from(1)
	r.Header.Set("X-Forwarded-For", "2001:db8:2:1::1")
	if !s.allowCall(r, "joe") {
		t.Fatal("another network")
	}
}

// Past a line's wake budget, one call in 5 minutes still wakes the phones.
func TestWakesGoOnPastTheBudget(t *testing.T) {
	s, _, _ := testSite(t)
	wake := func(i int) bool {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		return s.allowWake(r, "joe")
	}
	for i := 1; i <= s.Limits.LineCalls.Max; i++ {
		if !wake(i) {
			t.Fatalf("call %d didn't wake", i)
		}
	}
	if !wake(100) {
		t.Fatal("the first call past the budget should wake")
	}
	if wake(101) {
		t.Fatal("the next one shouldn't")
	}
}

// A line takes at most 60 messages a day, from any number of addresses.
func TestVoicemailsPerDay(t *testing.T) {
	s, _, _ := testSite(t)
	n := 0
	for i := 0; i < 80; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", "198.51.100.1")
		s.Limits.Voicemails = NewWindow(1000, time.Hour)
		if s.takeVoicemail(r, "joe") {
			n++
		}
	}
	if n != 60 {
		t.Fatalf("messages: %d", n)
	}
}

// A right password gives its count back, and a stranger with a few addresses
// can't lock the owner out.
func TestLoginOwnerStaysIn(t *testing.T) {
	_, st, h := testSite(t)
	st.CreateAccount("joe@example.com", "the-right-password", "")
	for i := 0; i < 8; i++ {
		if w := post(h, "/login", url.Values{"email": {"joe@example.com"}, "password": {"the-right-password"}}, ""); w.Code != http.StatusSeeOther {
			t.Fatalf("login %d: %d", i+1, w.Code)
		}
	}
	login := func(addr, password string) int {
		r := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"email": {"joe@example.com"}, "password": {password}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", addr)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, addr := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
		for i := 0; i < 5; i++ {
			login(addr, "wrong-password")
		}
	}
	if c := login("203.0.113.9", "the-right-password"); c != http.StatusSeeOther {
		t.Fatalf("the owner, from another address: %d", c)
	}
}

func TestSignupBurstIsLimited(t *testing.T) {
	_, st, h := testSite(t)
	if err := st.CreatePromoCode("FRIENDS-2027", store.Basic, 0, "", ""); err != nil {
		t.Fatal(err)
	}
	got := burst(30, func(i int) int {
		return post(h, "/signup", url.Values{"email": {"a" + strconv.Itoa(i) + "@example.com"}, "password": {"long-enough"}, "promo": {"FRIENDS-2027"}}, "").Code
	})
	if got[http.StatusSeeOther] != 3 {
		t.Fatalf("accounts made: %v", got)
	}
	got = burst(100, func(int) int {
		return post(h, "/signup", url.Values{"email": {"b@example.com"}, "password": {"long-enough"}, "promo": {"GUESS-1234"}}, "").Code
	})
	if got[http.StatusUnprocessableEntity] > 10 {
		t.Fatalf("code guesses checked: %v", got)
	}
}

// voicemailPass asks the real socket for a pass, as a visitor would.
func voicemailPass(t *testing.T, srv *httptest.Server, line, addr string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?role=visitor&line="+line,
		&websocket.DialOptions{HTTPHeader: http.Header{"X-Forwarded-For": {addr}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if err := wsjson.Write(ctx, c, calls.Message{Type: "voicemail"}); err != nil {
		t.Fatal(err)
	}
	var m calls.Message
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	return m.Token
}

func voicemailUpload(h http.Handler, token, addr string, size int) int {
	return voicemailUploadMS(h, token, addr, size, 0)
}

func voicemailUploadMS(h http.Handler, token, addr string, size int, ms int64) int {
	var buf bytes.Buffer
	m := multipart.NewWriter(&buf)
	m.WriteField("duration_ms", strconv.FormatInt(ms, 10))
	head := textproto.MIMEHeader{}
	head.Set("Content-Disposition", `form-data; name="audio"; filename="m.webm"`)
	head.Set("Content-Type", "audio/webm")
	part, _ := m.CreatePart(head)
	part.Write(make([]byte, size))
	m.Close()
	r := httptest.NewRequest("POST", "/api/voicemail", &buf)
	r.Header.Set("Content-Type", m.FormDataContentType())
	r.Header.Set(voicemailPassHeader, token)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", addr)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

// A Premium voicemail queues a note and wakes the worker; a Basic one and a very
// short one cost nothing; deleting the message deletes its note.
func TestVoicemailNotes(t *testing.T) {
	s, st, h := testSite(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	woken := 0
	s.OnNote = func() { woken++ }
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	st.CreateLine(u.AccountID, "joesbnb", "Joe's")
	leave := func(addr string, ms int64) {
		t.Helper()
		if c := voicemailUploadMS(h, voicemailPass(t, srv, "joesbnb", addr), addr, 1000, ms); c != http.StatusNoContent {
			t.Fatalf("upload: %d", c)
		}
	}
	notes := func() []store.Note {
		t.Helper()
		list, err := st.Notes(u.AccountID)
		if err != nil {
			t.Fatal(err)
		}
		return list
	}

	leave("198.51.100.1", 30000)
	if len(notes()) != 0 || woken != 0 {
		t.Fatalf("a Basic voicemail: %d notes, woken %d", len(notes()), woken)
	}
	st.SetPlan(u.AccountID, store.Premium)
	leave("198.51.100.2", 1500)
	if len(notes()) != 0 || woken != 0 {
		t.Fatalf("a very short voicemail: %d notes, woken %d", len(notes()), woken)
	}
	leave("198.51.100.3", 30000)
	list := notes()
	if len(list) != 1 || woken != 1 || list[0].Kind != store.NoteVoicemail || list[0].Status != store.NoteWaiting || list[0].Seconds != 30 || list[0].Language != store.French {
		t.Fatalf("a Premium voicemail: %+v, woken %d", list, woken)
	}
	if _, err := os.Stat(st.NoteAudioPath(list[0])); err != nil {
		t.Fatalf("the note doesn't find its recording: %v", err)
	}
	messages, _ := st.Voicemails(u.AccountID)
	if err := st.DeleteVoicemail(u.AccountID, messages[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(notes()) != 0 {
		t.Fatalf("the note outlived its message: %+v", notes())
	}
}

func TestVoicemailUploads(t *testing.T) {
	_, st, h := testSite(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	st.CreateLine(u.AccountID, "joesbnb", "Joe's")

	// No pass: refused before anything is read.
	if c := voicemailUpload(h, "no-such-pass", "198.51.100.1", 10); c != http.StatusForbidden {
		t.Fatalf("no pass: %d", c)
	}
	// Too big.
	if c := voicemailUpload(h, voicemailPass(t, srv, "joesbnb", "198.51.100.8"), "198.51.100.8", store.MaxVoicemailBytes+2<<20); c != http.StatusRequestEntityTooLarge {
		t.Fatalf("too big: %d", c)
	}
	// Passes stored up and sent at once: 3 go through, and each pass once.
	passes := []string{}
	for i := 0; i < 8; i++ {
		passes = append(passes, voicemailPass(t, srv, "joesbnb", "198.51.100.2"))
	}
	got := burst(len(passes), func(i int) int { return voicemailUpload(h, passes[i], "198.51.100.2", 100) })
	if got[http.StatusNoContent] > 3 {
		t.Fatalf("answers: %v", got)
	}
	list, _ := st.Voicemails(u.AccountID)
	if len(list) > 3 {
		t.Fatalf("%d messages saved", len(list))
	}
	// A visitor gets 10 passes an hour.
	last := ""
	for i := 0; i < 12; i++ {
		last = voicemailPass(t, srv, "joesbnb", "198.51.100.3")
	}
	if last != "" {
		t.Fatal("an 11th pass in an hour")
	}
}

func TestLongTextIsRefused(t *testing.T) {
	_, st, h := testSite(t)
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	st.CreateLine(u.AccountID, "joesbnb", "Joe's")
	token, _, _ := st.NewSession(u.ID)
	if w := post(h, "/dashboard/business", url.Values{"business": {strings.Repeat("x", 500)}}, token); w.Code != http.StatusSeeOther {
		t.Fatalf("status %d", w.Code)
	}
	l, _ := st.LineForAccount(u.AccountID)
	if len(l.Business) > 100 {
		t.Fatalf("the name is %d long", len(l.Business))
	}
	for _, form := range []url.Values{
		{"greeting": {strings.Repeat("x", 2001)}, "key": {"1"}, "label": {"Talk"}, "action": {"ring"}, "say": {""}, "url": {""}, "say_audio": {""}},
		{"greeting": {"Hi"}, "key": {"1"}, "label": {strings.Repeat("x", 81)}, "action": {"ring"}, "say": {""}, "url": {""}, "say_audio": {""}},
		{"greeting": {strings.Repeat("x", 200<<10)}, "key": {"1"}, "label": {"Talk"}, "action": {"ring"}, "say": {""}, "url": {""}, "say_audio": {""}},
	} {
		if w := post(h, "/dashboard/menu", form, token); w.Code < 400 {
			t.Errorf("status %d", w.Code)
		}
	}
	if w := post(h, "/signup", url.Values{"email": {strings.Repeat("x", 300) + "@example.com"}, "password": {"long-enough"}, "promo": {"X"}}, ""); !strings.Contains(w.Body.String(), "Enter your email") {
		t.Error("a very long email passed")
	}
	if validEmail("a@b.c?bcc=evil@example.com") || validEmail("<script>@example.com") || !validEmail("joe.smith+x@example.com") {
		t.Error("validEmail")
	}
}

// The relay isn't free for all: a stranger's browser gets only the address
// helper; a logged-in owner gets a login.
func TestRelayLogins(t *testing.T) {
	s, st, h := testSite(t)
	s.Relay = RelayInfo{Secret: "s3cret", Host: "voip.example", Port: 3478}
	get := func(cookie string) string {
		r := httptest.NewRequest("GET", "/api/ice", nil)
		if cookie != "" {
			r.Header.Set("Cookie", sessionCookie+"="+cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Body.String()
	}
	if b := get(""); strings.Contains(b, "credential") || !strings.Contains(b, "stun:voip.example") {
		t.Fatalf("a stranger: %s", b)
	}
	u, _ := st.CreateAccount("joe@example.com", "long-enough", "")
	token, _, _ := st.NewSession(u.ID)
	if b := get(token); !strings.Contains(b, "credential") {
		t.Fatalf("the owner: %s", b)
	}
	if !strings.Contains(string(s.relayLogin()), "credential") {
		t.Fatal("what a caller gets once answered")
	}
}

func TestPasswordChangeForgetsDevices(t *testing.T) {
	_, st, _ := testSite(t)
	u, _ := st.CreateAccount("joe@example.com", "old-password", "")
	st.CreateLine(u.AccountID, "joesbnb", "Joe's")
	st.SaveDevice(u.AccountID, store.Device{Endpoint: "fcm:stolen"})
	token, _, _ := st.NewSession(u.ID)
	if err := st.ChangePassword(u.ID, "old-password", "new-password", token); err != nil {
		t.Fatal(err)
	}
	if n := st.DeviceCount(u.AccountID); n != 0 {
		t.Fatalf("%d devices left", n)
	}
}
