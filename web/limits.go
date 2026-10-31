package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limits keep robots and pranksters from ringing an owner all night,
// guessing passwords, filling the voicemail or making accounts by the
// hundred. All are counted in memory per visitor address (clientIP) and
// forgotten on restart. Visitors on this machine or the home network aren't
// limited (the tests and Ken's own phone); see clientIP.
type Limits struct {
	Calls      *Window // a visitor ringing one line
	LineCalls  *Window // everyone ringing one line
	Knocks     *Window // a guest knocking on one line's meeting room
	Voicemails *Window // a visitor leaving messages on one line
	LineVMs    *Window // everyone leaving messages on one line
	Logins     *Window // wrong passwords from one visitor
	Signups    *Window // new accounts from one visitor
	Premium    *Window // "Get Premium" emails from one visitor
}

// NewLimits gives the limits agreed with Ken on 2026-10-28.
func NewLimits() *Limits {
	return &Limits{
		Calls:      NewWindow(3, 10*time.Minute),
		LineCalls:  NewWindow(20, time.Hour),
		Knocks:     NewWindow(3, 10*time.Minute),
		Voicemails: NewWindow(3, time.Hour),
		LineVMs:    NewWindow(20, time.Hour),
		Logins:     NewWindow(5, 15*time.Minute),
		Signups:    NewWindow(3, time.Hour),
		Premium:    NewWindow(3, time.Hour),
	}
}

// Window counts what each key did during the last Per; Max is allowed.
type Window struct {
	Max int
	Per time.Duration

	mu    sync.Mutex
	seen  map[string][]time.Time
	swept time.Time
}

func NewWindow(max int, per time.Duration) *Window {
	return &Window{Max: max, Per: per, seen: map[string][]time.Time{}}
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

// Full says whether key used up its window.
func (w *Window) Full(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.recent(key, time.Now())) >= w.Max
}

// Add counts one more for key.
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

// Take counts one more for key, unless it used up its window.
func (w *Window) Take(key string) bool {
	w.mu.Lock()
	full := len(w.recent(key, time.Now())) >= w.Max
	w.mu.Unlock()
	if full {
		return false
	}
	w.Add(key)
	return true
}

// clientIP is the visitor's address, and local is true when the visitor is
// on this machine or the home network. Behind Caddy (or the dev proxy) the
// connection comes from this machine and the visitor is in
// X-Forwarded-For; Caddy writes that header itself, so a visitor can't fake
// it. The last address that isn't local is the visitor.
func clientIP(r *http.Request) (ip string, local bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !isLocal(host) {
		return host, false
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		if hop := strings.TrimSpace(hops[i]); hop != "" && !isLocal(hop) {
			return hop, false
		}
	}
	return host, true
}

func isLocal(addr string) bool {
	ip := net.ParseIP(addr)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// ----------------------------------------------------- the checks, by place

// allowCall is the hub's AllowCall: a visitor rings a line 3 times in 10
// minutes, and a line rings 20 times an hour at most. Past that, the caller
// is offered voicemail instead.
func (s *Server) allowCall(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	if local {
		return true
	}
	if s.Limits.Calls.Full(ip+" "+line) || s.Limits.LineCalls.Full(line) {
		return false
	}
	s.Limits.Calls.Add(ip + " " + line)
	s.Limits.LineCalls.Add(line)
	return true
}

// allowKnock is the rooms' AllowKnock: past 3 knocks in 10 minutes the guest
// still waits, but the owner's phone isn't woken again.
func (s *Server) allowKnock(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	return local || s.Limits.Knocks.Take(ip+" "+line)
}

// voicemailFull says whether a visitor may not leave another message on the
// line: 3 an hour each, 20 an hour on a line. Counted when one is saved
// (voicemailSaved).
func (s *Server) voicemailFull(r *http.Request, line string) bool {
	ip, local := clientIP(r)
	return !local && (s.Limits.Voicemails.Full(ip+" "+line) || s.Limits.LineVMs.Full(line))
}

func (s *Server) voicemailSaved(r *http.Request, line string) {
	if ip, local := clientIP(r); !local {
		s.Limits.Voicemails.Add(ip + " " + line)
		s.Limits.LineVMs.Add(line)
	}
}

// loginBlocked: 5 wrong passwords in 15 minutes, and that visitor waits.
func (s *Server) loginBlocked(r *http.Request) bool {
	ip, local := clientIP(r)
	return !local && s.Limits.Logins.Full(ip)
}

func (s *Server) wrongPassword(r *http.Request) {
	if ip, local := clientIP(r); !local {
		s.Limits.Logins.Add(ip)
	}
}

// signupFull: 3 new accounts an hour from one visitor.
func (s *Server) signupFull(r *http.Request) bool {
	ip, local := clientIP(r)
	return !local && s.Limits.Signups.Full(ip)
}

func (s *Server) signedUp(r *http.Request) {
	if ip, local := clientIP(r); !local {
		s.Limits.Signups.Add(ip)
	}
}
