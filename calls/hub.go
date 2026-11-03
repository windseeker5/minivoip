// Package calls sets up calls between a visitor's browser and the owner's
// phone app. It only passes messages: the voice itself goes directly between
// the two browsers (WebRTC). Ported from the Python proof of concept, with
// the same message types.
//
// Visitor -> server: call, voicemail, offer, ice, hangup
// Owner   -> server: hello, accept, decline, answer, ice, hangup
// Server  -> visitor: ringing, busy, accepted, no-answer, voicemail, limited, answer, ice, hangup
// Server  -> owner:   ready, incoming, taken, missed, voicemail, offer, ice, hangup,
// room (who waits in the meeting room, room.go)
//
// "no-answer" and "voicemail" to the visitor carry a one-time token that lets
// the browser upload one voicemail (UseVoicemailToken). "limited" instead
// says this caller left enough messages for now (AllowVoicemail).
package calls

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Message is every message on the socket, in both directions.
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Option    string          `json:"option,omitempty"` // menu choice the visitor picked, shown on the phone
	SDP       string          `json:"sdp,omitempty"`
	Token     string          `json:"token,omitempty"` // voicemail upload pass
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Names     []string        `json:"names,omitempty"` // "room": the guests waiting in the meeting room
	ICE       json.RawMessage `json:"ice,omitempty"`   // "accepted": the connection helpers, with a relay login
}

type peer struct {
	*outbox
	owner bool
	req   *http.Request // who connected, for AllowCall and AllowVoicemail
	token string        // this socket's voicemail pass; a new one replaces it
}

func (p *peer) send(m Message) { p.put(m) }

type call struct {
	id       string
	option   string
	visitor  *peer
	owner    *peer // set when answered
	timer    *time.Timer
	stopRing func()
	started  time.Time
	answered time.Time
	noPush   bool // past the line's budget: open apps ring, sleeping phones aren't woken
}

// Record is a finished call, for the owner's call history.
type Record struct {
	ID      string
	Option  string
	Outcome string // answered, missed (rang out or the caller hung up), declined, away (the owner was set to away)
	Started time.Time
	// Answered is zero when nobody answered.
	Answered time.Time
	Ended    time.Time
}

func (c *call) quiet() {
	if c.stopRing != nil {
		c.stopRing()
	}
}

type line struct {
	owners  map[*peer]bool // open phone apps
	current *call          // the call that is ringing or active; one at a time
}

// Hub holds every line's open phone apps and current call.
type Hub struct {
	RingTimeout time.Duration
	// OriginPatterns lists the hosts allowed to open a socket besides the
	// page's own host (the dev proxy on :7331 sits in front of :8090).
	OriginPatterns []string
	// LineExists says whether a line name is real.
	LineExists func(name string) bool
	// CanAnswer says whether this request (its login) may be the phone app
	// for the line. Nil allows everyone (tests).
	CanAnswer func(r *http.Request, line string) bool
	// OnRing starts waking the owner's sleeping phones (push) and returns a
	// function that stops it. OnMissed says a call rang out, OnAnswered that
	// it was picked up. All are called with the hub locked, so they must not
	// block. Nil skips them.
	OnRing     func(line, callID, option string) (stop func())
	OnMissed   func(line, callID string)
	OnAnswered func(line, callID string)
	// OnEnd records a finished call (call history). It runs with the hub
	// locked, before the phone apps hear the call ended, so a phone that
	// reloads its history sees it. Nil skips it.
	OnEnd func(line string, r Record)
	// Available says whether the owner takes calls now (the phone app's
	// Available switch). When not, callers go straight to voicemail. Nil
	// means always.
	Available func(line string) bool
	// AllowCall says whether this caller may ring the line now (call
	// limits, web/limits.go); when not, they're offered voicemail without
	// ringing. AllowVoicemail says whether they may leave a message. Both
	// run with the hub locked. Nil allows everything.
	AllowCall      func(r *http.Request, line string) bool
	AllowVoicemail func(r *http.Request, line string) bool
	// CleanOption turns what a caller says they picked into the name of a
	// real choice on the line's menu, or "": it shows on the owner's screen
	// and in the history. It runs with the hub locked. Nil keeps it as is.
	CleanOption func(line, option string) string
	// AllowWake says whether this call may wake the owner's sleeping phones.
	// When not, it only rings open apps; with none open the caller is offered
	// voicemail. It runs with the hub locked. Nil always wakes them.
	AllowWake func(r *http.Request, line string) bool
	// ICE gives a caller whose call was answered the connection helpers, with
	// a relay login. It runs with the hub locked. Nil sends none.
	ICE func() json.RawMessage

	mu        sync.Mutex
	lines     map[string]*line
	vmTokens  map[string]vmToken
	nextSweep time.Time
}

type vmToken struct {
	VoicemailPass
	expires time.Time
}

// VoicemailPass says what a voicemail upload is for.
type VoicemailPass struct {
	Line   string
	Option string
	CallID string // the unanswered call it follows; "" for a voicemail choice
}

// voicemailWindow is how long a caller has to record and send a message.
const voicemailWindow = 5 * time.Minute

func NewHub(lineExists func(name string) bool) *Hub {
	return &Hub{RingTimeout: 30 * time.Second, LineExists: lineExists, lines: map[string]*line{}, vmTokens: map[string]vmToken{}}
}

func (h *Hub) line(name string) *line {
	l := h.lines[name]
	if l == nil {
		l = &line{owners: map[*peer]bool{}}
		h.lines[name] = l
	}
	return l
}

// OwnersOnline counts the line's open phone apps.
func (h *Hub) OwnersOnline(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if l := h.lines[name]; l != nil {
		return len(l.owners)
	}
	return 0
}

// ServeHTTP is the socket at /ws?line=<name>&role=visitor|owner.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("line")
	role := r.URL.Query().Get("role")
	if !h.LineExists(name) || (role != "visitor" && role != "owner") {
		http.Error(w, "unknown line or role", http.StatusBadRequest)
		return
	}
	if role == "owner" && h.CanAnswer != nil && !h.CanAnswer(r, name) {
		http.Error(w, "log in to answer this line", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.OriginPatterns})
	if err != nil {
		return
	}
	me := &peer{outbox: newOutbox(conn), owner: role == "owner", req: r}
	if me.owner {
		h.mu.Lock()
		h.line(name).owners[me] = true
		n := len(h.line(name).owners)
		h.mu.Unlock()
		log.Printf("calls: %s: phone app connected (%d open)", name, n)
	}
	defer h.leave(name, me)
	if !me.owner {
		// A visitor's socket that isn't in a call doesn't stay open for ever.
		stop := h.watchIdle(name, me)
		defer stop()
	}

	var speed pace
	for {
		var m Message
		if err := wsjson.Read(r.Context(), conn, &m); err != nil {
			return
		}
		if !speed.ok() {
			log.Printf("calls: %s: too many messages from one socket, closing it", name)
			return
		}
		me.touch()
		h.handle(name, me, m)
	}
}

func (h *Hub) handle(name string, me *peer, m Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.line(name)
	c := l.current
	if c != nil && m.ID != "" && m.ID != c.id {
		c = nil // a message about an older call
	}
	if h.CleanOption != nil && !me.owner && (m.Type == "call" || m.Type == "voicemail") {
		m.Option = h.CleanOption(name, m.Option)
	}

	switch {
	case !me.owner && m.Type == "call":
		if l.current != nil {
			me.send(Message{Type: "busy"})
			return
		}
		if h.AllowCall != nil && !h.AllowCall(me.req, name) {
			// Too many calls: no ring and no call history, just voicemail.
			log.Printf("calls: %s: call limit reached, sent to voicemail", name)
			me.send(h.offerVoicemail(name, me, "no-answer", m.Option, newID()))
			return
		}
		if h.Available != nil && !h.Available(name) {
			now := time.Now()
			id := newID()
			h.record(name, Record{ID: id, Option: m.Option, Outcome: "away", Started: now, Ended: now})
			log.Printf("calls: %s: call %s while away, sent to voicemail", name, id)
			me.send(h.offerVoicemail(name, me, "no-answer", m.Option, id))
			return
		}
		wake := h.AllowWake == nil || h.AllowWake(me.req, name)
		if !wake && len(l.owners) == 0 {
			log.Printf("calls: %s: too many callers, sent to voicemail", name)
			me.send(h.offerVoicemail(name, me, "no-answer", m.Option, newID()))
			return
		}
		c = &call{id: newID(), option: m.Option, visitor: me, started: time.Now(), noPush: !wake}
		l.current = c
		c.timer = time.AfterFunc(h.RingTimeout, func() { h.noAnswer(name, c) })
		if h.OnRing != nil && wake {
			c.stopRing = h.OnRing(name, c.id, c.option)
		}
		log.Printf("calls: %s: call %s, %d phone app(s) open", name, c.id, len(l.owners))
		me.send(Message{Type: "ringing", ID: c.id})
		for o := range l.owners {
			o.send(Message{Type: "incoming", ID: c.id, Option: c.option})
		}

	case !me.owner && m.Type == "voicemail":
		// The caller picked a "take a voicemail" choice.
		me.send(h.offerVoicemail(name, me, "voicemail", m.Option, ""))

	case me.owner && m.Type == "hello":
		me.send(Message{Type: "ready"})
		if l.current != nil && l.current.owner == nil {
			me.send(Message{Type: "incoming", ID: l.current.id, Option: l.current.option})
		}

	case me.owner && m.Type == "accept":
		if c == nil || c.owner != nil {
			me.send(Message{Type: "missed", ID: m.ID})
			return
		}
		c.owner = me
		c.answered = time.Now()
		c.timer.Stop()
		c.quiet()
		if h.OnAnswered != nil && !c.noPush {
			h.OnAnswered(name, c.id)
		}
		log.Printf("calls: %s: call %s answered", name, c.id)
		for o := range l.owners {
			if o != me {
				o.send(Message{Type: "taken", ID: c.id})
			}
		}
		accepted := Message{Type: "accepted", ID: c.id}
		if h.ICE != nil {
			accepted.ICE = h.ICE()
		}
		c.visitor.send(accepted)

	case me.owner && m.Type == "decline":
		if c != nil && c.owner == nil {
			h.endUnanswered(name, c, "declined")
		}

	case m.Type == "offer" || m.Type == "answer" || m.Type == "ice":
		if c == nil || c.owner == nil {
			return
		}
		switch me {
		case c.visitor:
			c.owner.send(m)
		case c.owner:
			c.visitor.send(m)
		}

	case m.Type == "hangup":
		if c != nil && (me == c.visitor || me == c.owner) {
			h.end(name, c, me)
		}
	}
}

// Decline turns down a ringing call from outside the socket (the Android
// app's ring screen). It reports whether that call was ringing.
func (h *Hub) Decline(name, callID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.line(name).current
	if c == nil || c.id != callID || c.owner != nil {
		return false
	}
	h.endUnanswered(name, c, "declined")
	return true
}

// leave runs when a socket closes: it ends that peer's call.
func (h *Hub) leave(name string, me *peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.line(name)
	delete(l.owners, me)
	if c := l.current; c != nil && (c.visitor == me || c.owner == me) {
		h.end(name, c, me)
	}
	me.stop() // a voicemail pass stays until it expires: the upload may be on its way
}

// Recheck closes the line's phone apps whose login ended (logged out, or the
// password changed).
func (h *Hub) Recheck(name string) {
	if h.CanAnswer == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for o := range h.line(name).owners {
		if !h.CanAnswer(o.req, name) {
			o.kill()
		}
	}
}

// RecheckAll runs Recheck on every line with an open phone app, for logins
// that ran out while their socket stayed open.
func (h *Hub) RecheckAll() {
	h.mu.Lock()
	names := make([]string, 0, len(h.lines))
	for name, l := range h.lines {
		if len(l.owners) > 0 {
			names = append(names, name)
		}
	}
	h.mu.Unlock()
	for _, name := range names {
		h.Recheck(name)
	}
}

// idleLimit is how long a visitor's socket stays open without a call or a
// message. The call window opens its socket when the caller presses Call.
const idleLimit = 15 * time.Minute

// watchIdle closes a visitor's socket left doing nothing.
func (h *Hub) watchIdle(name string, me *peer) (stop func()) {
	t := time.NewTicker(time.Minute)
	done := make(chan struct{})
	go func() {
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				h.mu.Lock()
				c := h.line(name).current
				busy := c != nil && c.visitor == me
				h.mu.Unlock()
				if !busy && me.idleFor() > idleLimit {
					me.kill()
					return
				}
			}
		}
	}()
	return func() { close(done) }
}

func (h *Hub) noAnswer(name string, c *call) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.line(name).current == c && c.owner == nil {
		h.endUnanswered(name, c, "missed")
	}
}

// endUnanswered stops the ringing. The visitor is offered voicemail.
// outcome is "missed" (rang out) or "declined". Caller holds h.mu.
func (h *Hub) endUnanswered(name string, c *call, outcome string) {
	l := h.line(name)
	c.timer.Stop()
	c.quiet()
	if h.OnMissed != nil && !c.noPush {
		h.OnMissed(name, c.id)
	}
	h.record(name, Record{ID: c.id, Option: c.option, Outcome: outcome, Started: c.started, Ended: time.Now()})
	l.current = nil
	log.Printf("calls: %s: call %s not answered", name, c.id)
	c.visitor.send(h.offerVoicemail(name, c.visitor, "no-answer", c.option, c.id))
	for o := range l.owners {
		o.send(Message{Type: "missed", ID: c.id})
	}
}

// end hangs up a ringing or active call and tells the other side.
// Caller holds h.mu.
func (h *Hub) end(name string, c *call, by *peer) {
	l := h.line(name)
	c.timer.Stop()
	c.quiet()
	if c.owner == nil && h.OnMissed != nil && !c.noPush {
		h.OnMissed(name, c.id)
	}
	r := Record{ID: c.id, Option: c.option, Outcome: "missed", Started: c.started, Answered: c.answered, Ended: time.Now()}
	if c.owner != nil {
		r.Outcome = "answered"
	}
	h.record(name, r)
	l.current = nil
	log.Printf("calls: %s: call %s ended", name, c.id)
	if by != c.visitor {
		c.visitor.send(Message{Type: "hangup", ID: c.id})
	}
	if c.owner != nil && by != c.owner {
		c.owner.send(Message{Type: "hangup", ID: c.id})
	}
	if c.owner == nil {
		for o := range l.owners {
			o.send(Message{Type: "missed", ID: c.id})
		}
	}
}

func (h *Hub) record(name string, r Record) {
	if h.OnEnd != nil {
		h.OnEnd(name, r)
	}
}

// offerVoicemail is the message that lets a caller record a voicemail
// (typ "no-answer" or "voicemail", with a pass), or "limited" when they left
// enough messages for now. Caller holds h.mu.
func (h *Hub) offerVoicemail(name string, to *peer, typ, option, callID string) Message {
	if h.AllowVoicemail != nil && !h.AllowVoicemail(to.req, name) {
		log.Printf("calls: %s: voicemail limit reached", name)
		return Message{Type: "limited", ID: callID}
	}
	// One pass per socket: asking again replaces it.
	delete(h.vmTokens, to.token)
	to.token = h.issueToken(name, option, callID)
	return Message{Type: typ, ID: callID, Token: to.token}
}

// issueToken makes a one-time voicemail pass. Caller holds h.mu.
func (h *Hub) issueToken(line, option, callID string) string {
	now := time.Now()
	if now.After(h.nextSweep) {
		h.nextSweep = now.Add(time.Minute)
		for t, v := range h.vmTokens {
			if now.After(v.expires) {
				delete(h.vmTokens, t)
			}
		}
	}
	t := newID() + newID()
	h.vmTokens[t] = vmToken{VoicemailPass: VoicemailPass{Line: line, Option: option, CallID: callID}, expires: now.Add(voicemailWindow)}
	return t
}

// VoicemailTokenValid says whether a voicemail pass is good, without spending
// it: an upload is checked before its body is read, and spent once it arrived,
// so a caller on a bad connection can try again.
func (h *Hub) VoicemailTokenValid(token string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, found := h.vmTokens[token]
	return found && time.Now().Before(v.expires)
}

// UseVoicemailToken spends a voicemail pass and says what it was for.
func (h *Hub) UseVoicemailToken(token string) (VoicemailPass, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, found := h.vmTokens[token]
	delete(h.vmTokens, token)
	if !found || time.Now().After(v.expires) {
		return VoicemailPass{}, false
	}
	return v.VoicemailPass, true
}

// NotifyOwners sends a message to every open phone app of a line.
func (h *Hub) NotifyOwners(line string, m Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for o := range h.line(line).owners {
		o.send(m)
	}
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
