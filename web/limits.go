package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"minivoip/push"
)

// Limits keep robots and pranksters from ringing an owner all night,
// guessing passwords or promo codes, listing lines, filling the voicemail or
// making accounts by the hundred. All are counted in memory per visitor
// address (clientIP) and forgotten on restart. Visitors on this machine or
// the home network aren't limited (the tests and Ken's own phone); see
// clientIP.
type Limits struct {
	Calls       *Window // a visitor ringing one line
	CallsDay    *Window // the same, over a day
	LineCalls   *Window // visitors ringing one line, each network counted once
	LineSeen    *Window // which networks were counted in LineCalls
	Knocks      *Window // a guest knocking on one line's meeting room
	LineKnocks  *Window // everyone knocking on one line's meeting room
	Voicemails  *Window // a visitor leaving messages on one line
	LineVMs     *Window // visitors leaving messages on one line, each network counted once
	LineVMSeen  *Window // which networks were counted in LineVMs
	LineVMDay   *Window // every message left on one line, over a day
	Passes      *Window // voicemail passes given to a visitor on one line
	Uploads     *Gauge  // voicemail uploads under way from one visitor
	Logins      *Window // wrong passwords from one visitor
	Accounts    *Window // wrong passwords on one account, from anywhere
	PushTests   *Window // "Send a test" from one account
	Signups     *Window // new accounts from one visitor
	SignupTries *Window // refused sign ups from one visitor: guessing codes or emails
	Premium     *Window // "Get Premium" emails from one visitor
	Lookups     *Window // call pages, status checks and sockets from one visitor
	Relay       *Window // relay logins given to one visitor
	Alerts      *Window // "too many calls" notices to one line's owner
	LateWakes   *Window // calls that still wake a line's phones past its budget
	Sockets     *Gauge  // sockets open from one visitor
}

// NewLimits gives the limits agreed with Ken on 2026-10-28, and the ones the
// security review added on 2026-11-02.
func NewLimits() *Limits {
	return &Limits{
		Calls:       NewWindow(3, 10*time.Minute).perNetwork(3),
		CallsDay:    NewWindow(12, 24*time.Hour).perNetwork(3),
		LineCalls:   NewWindow(20, time.Hour),
		LineSeen:    NewWindow(1, time.Hour),
		Knocks:      NewWindow(3, 10*time.Minute).perNetwork(3),
		LineKnocks:  NewWindow(20, time.Hour),
		Voicemails:  NewWindow(3, time.Hour).perNetwork(3),
		LineVMDay:   NewWindow(60, 24*time.Hour),
		LineVMs:     NewWindow(20, time.Hour),
		LineVMSeen:  NewWindow(1, time.Hour),
		Passes:      NewWindow(10, time.Hour).perNetwork(3),
		Uploads:     NewGauge(2).perNetwork(5),
		Logins:      NewWindow(5, 15*time.Minute).perNetwork(10),
		Accounts:    NewWindow(100, 15*time.Minute),
		PushTests:   NewWindow(10, time.Hour),
		Signups:     NewWindow(3, time.Hour).perNetwork(10),
		SignupTries: NewWindow(10, time.Hour).perNetwork(10),
		Premium:     NewWindow(3, time.Hour).perNetwork(10),
		Lookups:     NewWindow(300, 10*time.Minute).perNetwork(10),
		Relay:       NewWindow(30, 10*time.Minute).perNetwork(10),
		Alerts:      NewWindow(1, time.Hour),
		LateWakes:   NewWindow(1, 5*time.Minute),
		Sockets:     NewGauge(20).perNetwork(5),
	}
}

// Gauge counts what each key has open right now; Max is allowed.
type Gauge struct {
	Max int

	wide *Gauge // the same count for a whole IPv6 network (perNetwork)

	mu   sync.Mutex
	open map[string]int
}

func NewGauge(max int) *Gauge { return &Gauge{Max: max, open: map[string]int{}} }

// perNetwork also caps a whole IPv6 network at this many times Max.
func (g *Gauge) perNetwork(times int) *Gauge {
	g.wide = NewGauge(g.Max * times)
	return g
}

// Take counts one more for key, unless it's at Max. Give it back with Done.
func (g *Gauge) Take(key string) bool {
	wide, isV6 := wideKey(key)
	isV6 = isV6 && g.wide != nil
	if isV6 && !g.wide.Take(wide) {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open[key] >= g.Max {
		if isV6 {
			g.wide.Done(wide)
		}
		return false
	}
	g.open[key]++
	return true
}

func (g *Gauge) Done(key string) {
	if wide, isV6 := wideKey(key); isV6 && g.wide != nil {
		g.wide.Done(wide)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open[key] > 1 {
		g.open[key]--
	} else {
		delete(g.open, key)
	}
}

// Window counts what each key did during the last Per; Max is allowed.
type Window struct {
	Max int
	Per time.Duration

	wide *Window // the same count for a whole IPv6 network (perNetwork)

	mu    sync.Mutex
	seen  map[string][]time.Time
	swept time.Time
}

func NewWindow(max int, per time.Duration) *Window {
	return &Window{Max: max, Per: per, seen: map[string][]time.Time{}}
}

// perNetwork also caps a whole IPv6 network (/48) at this many times Max: one
// customer can hold thousands of /64s, and each would start from zero.
func (w *Window) perNetwork(times int) *Window {
	w.wide = NewWindow(w.Max*times, w.Per)
	return w
}

// wideKey is a visitor's key with their /64 widened to its /48. A key is an
// address, or an address, a space and a line name.
func wideKey(key string) (string, bool) {
	addr, rest, _ := strings.Cut(key, " ")
	if !strings.HasSuffix(addr, "/64") {
		return "", false
	}
	return networkKey(addr) + " " + rest, true
}

// recent drops what is older than the window. Caller holds w.mu.
func (w *Window) recent(key string, now time.Time) []time.Time {
	list := w.seen[key]
	i := 0
	for i < len(list) && now.Sub(list[i]) >= w.Per {
		i++
	}
	list = list[i:]
	if len(list) == 0 {
		delete(w.seen, key)
	} else {
		w.seen[key] = list
	}
	return list
}

// Used says whether key did anything during the window.
func (w *Window) Used(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.recent(key, time.Now())) > 0
}

// Full says whether key used up its window.
func (w *Window) Full(key string) bool {
	if wide, isV6 := wideKey(key); isV6 && w.wide != nil && w.wide.Full(wide) {
		return true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.recent(key, time.Now())) >= w.Max
}

// Add counts one more for key, whatever the window says.
func (w *Window) Add(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	w.seen[key] = append(w.recent(key, now), now)
	// Now and then forget the keys that went quiet.
	if now.Sub(w.swept) > w.Per {
		w.swept = now
		for k := range w.seen {
			w.recent(k, now)
		}
	}
}

// Take counts one more for key, unless it used up its window. Check and count
// are one step: requests arriving at the same moment can't all slip through.
func (w *Window) Take(key string) bool {
	wide, isV6 := wideKey(key)
	isV6 = isV6 && w.wide != nil
	if isV6 && !w.wide.Take(wide) {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if len(w.recent(key, now)) >= w.Max {
		if isV6 {
			w.wide.Give(wide)
		}
		return false
	}
	w.seen[key] = append(w.seen[key], now)
	if now.Sub(w.swept) > w.Per {
		w.swept = now
		for k := range w.seen {
			w.recent(k, now)
		}
	}
	return true
}

// Give takes back the last Take: what it counted turned out fine, or never
// happened.
func (w *Window) Give(key string) {
	if wide, isV6 := wideKey(key); isV6 && w.wide != nil {
		w.wide.Give(wide)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if list := w.seen[key]; len(list) > 1 {
		w.seen[key] = list[:len(list)-1]
	} else {
		delete(w.seen, key)
	}
}

// TrustedProxies are the addresses or networks (like 172.20.0.0/16) of the
// proxies in front of this server besides this machine
// (MINIVOIP_TRUSTED_PROXY, for a container whose Caddy is elsewhere). Only they
// may say who the visitor is.
var TrustedProxies []string

// ExemptHomeNetwork lifts the limits for visitors on private addresses, like
// Ken's phone on the home Wi-Fi while developing. This machine is always
// exempt. Off in a container, where a private address proves nothing.
var ExemptHomeNetwork bool

func trustedProxy(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, p := range TrustedProxies {
		p = strings.TrimSpace(p)
		if q := net.ParseIP(p); q != nil && q.Equal(ip) {
			return true
		}
		if _, n, err := net.ParseCIDR(p); err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP is the visitor's address, and local is true when the visitor is
// on this machine or the home network. Behind Caddy (or the dev proxy) the
// connection comes from a trusted proxy and the visitor is in
// X-Forwarded-For; Caddy writes that header itself, so a visitor can't fake
// it. The last address that isn't local is the visitor. Anyone connecting
// directly is just who they are, even from a private address: a container's
// published port looks private and must not switch the limits off.
func clientIP(r *http.Request) (ip string, local bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !trustedProxy(host) {
		return visitorKey(host), false
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if hop == "" || trustedProxy(hop) || ExemptHomeNetwork && isLocal(hop) {
			continue
		}
		return visitorKey(hop), false
	}
	// No visitor named. Only this machine itself is exempt: another proxy
	// that names nobody must not switch the limits off for everyone behind it.
	if ip := net.ParseIP(host); ExemptHomeNetwork || ip != nil && ip.IsLoopback() {
		return host, true
	}
	return visitorKey(host), false
}

func isLocal(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// visitorKey is what a visitor is counted by: their address, or for IPv6 their
// network (/64). One home gets a whole /64, so single addresses mean nothing.
func visitorKey(addr string) string {
	ip := net.ParseIP(addr)
	if ip == nil {
		return addr
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// networkKey groups visitors a bit wider, for the budgets a whole line shares:
// one IPv6 customer can hold a /48, thousands of /64s.
func networkKey(key string) string {
	ip := net.ParseIP(strings.TrimSuffix(key, "/64"))
	if ip == nil || ip.To4() != nil {
		return key
	}
	return ip.Mask(net.CIDRMask(48, 128)).String() + "/48"
}

// ----------------------------------------------------- the checks, by place

// allowCall is the hub's AllowCall: a visitor calls a line 3 times in 10
// minutes and 12 times a day. Past that, the caller is offered voicemail.
func (s *Server) allowCall(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	if local {
		return true
	}
	key := ip + " " + line
	if !s.Limits.Calls.Take(key) {
		return false
	}
	if !s.Limits.CallsDay.Take(key) {
		s.Limits.Calls.Give(key)
		return false
	}
	return true
}

// allowWake is the hub's AllowWake: 20 different networks wake a line's phones
// in an hour. Past that, one call in 5 minutes still does, and the others ring
// the owner's open apps, so a few pranksters can't silence a line or ring it all
// night, and the owner hears that it happened.
func (s *Server) allowWake(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	if local {
		return true
	}
	seen := networkKey(ip) + " " + line
	if s.Limits.LineCalls.Full(line) {
		if s.Push != nil && s.Limits.Alerts.Take(line) {
			go s.Push.ToLine(line, push.Notification{Kind: "limit", Title: "Many calls at once",
				Body: "More callers than usual: some went straight to voicemail without ringing your phone.",
				URL:  "/app", Tag: "limit"}, time.Hour, false)
		}
		return s.Limits.LateWakes.Take(line)
	}
	if s.Limits.LineSeen.Take(seen) {
		s.Limits.LineCalls.Add(line)
	}
	return true
}

// cleanOption is the hub's CleanOption: a choice really on the line's menu.
func (s *Server) cleanOption(line, option string) string {
	l, err := s.Store.LineByName(line)
	if err != nil {
		return ""
	}
	for _, o := range l.Options {
		if o.Label == option {
			return option
		}
	}
	return ""
}

// allowKnock is the rooms' AllowKnock: past 3 knocks in 10 minutes from one
// guest, or 20 in an hour on the line, the guest still waits but the owner's
// phone isn't woken again.
func (s *Server) allowKnock(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	return local || !s.Limits.LineKnocks.Full(line) && s.Limits.Knocks.Take(ip+" "+line) && s.Limits.LineKnocks.Take(line)
}

// voicemailFull says whether a visitor may not leave another message on the
// line: 3 an hour each, 20 networks an hour and 60 messages a day on a line, so
// filling a mailbox takes days and the owner sees it coming, and room in the
// mailbox.
func (s *Server) voicemailFull(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	if !local {
		seen := networkKey(ip) + " " + line
		if s.Limits.Voicemails.Full(ip+" "+line) || s.Limits.LineVMDay.Full(line) || s.Limits.LineVMs.Full(line) && !s.Limits.LineVMSeen.Used(seen) {
			return true
		}
	}
	return s.Store.MailboxFull(line)
}

// allowVoicemail is the hub's AllowVoicemail: the checks above, and 10 passes
// an hour for a visitor on a line, so they can't be stored up for one burst.
func (s *Server) allowVoicemail(r *http.Request, line string) bool {
	if s.voicemailFull(r, line) {
		return false
	}
	ip, local := clientIP(r)
	return local || s.Limits.Passes.Take(ip+" "+line)
}

// takeVoicemail counts a message before it is saved, in one step; give it back
// with giveVoicemail if saving fails.
func (s *Server) takeVoicemail(r *http.Request, line string) bool {
	if s.Store.MailboxFull(line) {
		return false
	}
	ip, local := clientIP(r)
	if local {
		return true
	}
	if !s.Limits.Voicemails.Take(ip + " " + line) {
		return false
	}
	if !s.Limits.LineVMDay.Take(line) {
		s.Limits.Voicemails.Give(ip + " " + line)
		return false
	}
	seen := networkKey(ip) + " " + line
	if s.Limits.LineVMSeen.Used(seen) {
		return true
	}
	if !s.Limits.LineVMs.Take(line) {
		s.Limits.Voicemails.Give(ip + " " + line)
		s.Limits.LineVMDay.Give(line)
		return false
	}
	s.Limits.LineVMSeen.Take(seen)
	return true
}

func (s *Server) giveVoicemail(r *http.Request, line string) {
	if ip, local := clientIP(r); !local {
		s.Limits.Voicemails.Give(ip + " " + line)
		s.Limits.LineVMDay.Give(line)
	}
}

// knownKey signs the known-browser mark; it is made once and kept in settings.
func (s *Server) knownKey() string {
	key, _ := s.Store.Setting("known_key")
	if key == "" {
		b := make([]byte, 32)
		rand.Read(b)
		key = hex.EncodeToString(b)
		s.Store.SetSetting("known_key", key)
		key, _ = s.Store.Setting("known_key")
	}
	return key
}

// knownMark is tied to one account and its current password: a new password
// ends every mark but the one given with it. "" for an unknown email.
func (s *Server) knownMark(email string) string {
	secret := s.Store.LoginSecret(email)
	if secret == "" {
		return ""
	}
	m := hmac.New(sha256.New, []byte(s.knownKey()))
	m.Write([]byte(secret))
	return hex.EncodeToString(m.Sum(nil))
}

// markKnown remembers for a year that this browser has logged in to this
// account. Strangers can use up an account's wrong passwords, but that never
// locks out its owner on their own phone or computer.
func (s *Server) markKnown(w http.ResponseWriter, r *http.Request, email string) {
	setCookie(w, r, knownCookie, s.knownMark(email), time.Now().AddDate(1, 0, 0))
}

func (s *Server) isKnown(r *http.Request, email string) bool {
	mark, ok := readCookie(r, knownCookie)
	if !ok {
		return false
	}
	want := s.knownMark(email)
	return want != "" && hmac.Equal([]byte(mark), []byte(want))
}

// takeLogin counts a password check before it runs, in one step: 5 from one
// visitor in 15 minutes, and 100 on one account from any number of addresses.
// A right password gives it back (loginWorked). The account limit is high so a
// stranger with a few addresses can't lock its owner out.
func (s *Server) takeLogin(r *http.Request, email string) bool {
	ip, local := clientIP(r)
	if local {
		return true
	}
	if !s.Limits.Logins.Take(ip) {
		return false
	}
	if !s.isKnown(r, email) && !s.Limits.Accounts.Take(strings.ToLower(email)) {
		s.Limits.Logins.Give(ip)
		return false
	}
	return true
}

func (s *Server) loginWorked(r *http.Request, email string) {
	if ip, local := clientIP(r); !local {
		s.Limits.Logins.Give(ip)
		if !s.isKnown(r, email) {
			s.Limits.Accounts.Give(strings.ToLower(email))
		}
	}
}

// takeSignup counts a sign up before it runs, in one step: 10 tries an hour
// from one visitor, so promo codes and emails can't be guessed, and 3 new
// accounts. signupDone settles it: an account made gives the try back, and a
// refusal gives the account back.
func (s *Server) takeSignup(r *http.Request) bool {
	ip, local := clientIP(r)
	if local {
		return true
	}
	if !s.Limits.SignupTries.Take(ip) {
		return false
	}
	if !s.Limits.Signups.Take(ip) {
		s.Limits.SignupTries.Give(ip)
		return false
	}
	return true
}

func (s *Server) signupDone(r *http.Request, made bool) {
	ip, local := clientIP(r)
	if local {
		return
	}
	if made {
		s.Limits.SignupTries.Give(ip)
	} else {
		s.Limits.Signups.Give(ip)
	}
}

// lookupFull: 300 call pages, status checks and sockets in 10 minutes from one
// visitor. Plenty for real use (the open page checks every 30 seconds), and
// listing every line name would take weeks.
func (s *Server) lookupFull(r *http.Request) bool {
	ip, local := clientIP(r)
	return !local && !s.Limits.Lookups.Take(ip)
}

// socket guards the call and room sockets: the lookup limit, and at most 20
// open from one visitor.
func (s *Server) socket(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, local := clientIP(r)
		if !local {
			if s.lookupFull(r) || !s.Limits.Sockets.Take(ip) {
				http.Error(w, "Too many connections.", http.StatusTooManyRequests)
				return
			}
			defer s.Limits.Sockets.Done(ip)
		}
		next.ServeHTTP(w, r)
	})
}
