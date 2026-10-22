// Package calls sets up calls between a visitor's browser and the owner's
// phone app. It only passes messages: the voice itself goes directly between
// the two browsers (WebRTC). Ported from the Python proof of concept, with
// the same message types.
//
// Visitor -> server: call, offer, ice, hangup
// Owner   -> server: hello, accept, decline, answer, ice, hangup
// Server  -> visitor: ringing, busy, accepted, no-answer, answer, ice, hangup
// Server  -> owner:   incoming, taken, missed, offer, ice, hangup
package calls

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"minivoip/lines"
)

// Message is every message on the socket, in both directions.
type Message struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Option    string          `json:"option,omitempty"` // menu choice the visitor picked, shown on the phone
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

type peer struct {
	conn  *websocket.Conn
	owner bool
}

func (p *peer) send(m Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, p.conn, m); err != nil {
		log.Printf("calls: send %s: %v", m.Type, err)
	}
}

type call struct {
	id      string
	option  string
	visitor *peer
	owner   *peer // set when answered
	timer   *time.Timer
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

	mu    sync.Mutex
	lines map[string]*line
}

func NewHub() *Hub {
	return &Hub{RingTimeout: 30 * time.Second, lines: map[string]*line{}}
}

func (h *Hub) line(name string) *line {
	l := h.lines[name]
	if l == nil {
		l = &line{owners: map[*peer]bool{}}
		h.lines[name] = l
	}
	return l
}

// ServeHTTP is the socket at /ws?line=<name>&role=visitor|owner.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("line")
	role := r.URL.Query().Get("role")
	if _, ok := lines.Get(name); !ok || (role != "visitor" && role != "owner") {
		http.Error(w, "unknown line or role", http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.OriginPatterns})
	if err != nil {
		return
	}
	me := &peer{conn: conn, owner: role == "owner"}
	if me.owner {
		h.mu.Lock()
		h.line(name).owners[me] = true
		n := len(h.line(name).owners)
		h.mu.Unlock()
		log.Printf("calls: %s: phone app connected (%d open)", name, n)
	}
	defer h.leave(name, me)

	for {
		var m Message
		if err := wsjson.Read(r.Context(), conn, &m); err != nil {
			return
		}
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

	switch {
	case !me.owner && m.Type == "call":
		if l.current != nil {
			me.send(Message{Type: "busy"})
			return
		}
		c = &call{id: newID(), option: m.Option, visitor: me}
		l.current = c
		c.timer = time.AfterFunc(h.RingTimeout, func() { h.noAnswer(name, c) })
		log.Printf("calls: %s: call %s (%s), %d phone app(s) open", name, c.id, c.option, len(l.owners))
		me.send(Message{Type: "ringing", ID: c.id})
		for o := range l.owners {
			o.send(Message{Type: "incoming", ID: c.id, Option: c.option})
		}

	case me.owner && m.Type == "hello":
		if l.current != nil && l.current.owner == nil {
			me.send(Message{Type: "incoming", ID: l.current.id, Option: l.current.option})
		}

	case me.owner && m.Type == "accept":
		if c == nil || c.owner != nil {
			me.send(Message{Type: "missed", ID: m.ID})
			return
		}
		c.owner = me
		c.timer.Stop()
		log.Printf("calls: %s: call %s answered", name, c.id)
		for o := range l.owners {
			if o != me {
				o.send(Message{Type: "taken", ID: c.id})
			}
		}
		c.visitor.send(Message{Type: "accepted", ID: c.id})

	case me.owner && m.Type == "decline":
		if c != nil && c.owner == nil {
			h.endUnanswered(name, c)
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

// leave runs when a socket closes: it ends that peer's call.
func (h *Hub) leave(name string, me *peer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	l := h.line(name)
	delete(l.owners, me)
	if c := l.current; c != nil && (c.visitor == me || c.owner == me) {
		h.end(name, c, me)
	}
	me.conn.CloseNow()
}

func (h *Hub) noAnswer(name string, c *call) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.line(name).current == c && c.owner == nil {
		h.endUnanswered(name, c)
	}
}

// endUnanswered stops the ringing. The visitor is offered voicemail.
// Caller holds h.mu.
func (h *Hub) endUnanswered(name string, c *call) {
	l := h.line(name)
	c.timer.Stop()
	l.current = nil
	log.Printf("calls: %s: call %s not answered", name, c.id)
	c.visitor.send(Message{Type: "no-answer", ID: c.id})
	for o := range l.owners {
		o.send(Message{Type: "missed", ID: c.id})
	}
}

// end hangs up a ringing or active call and tells the other side.
// Caller holds h.mu.
func (h *Hub) end(name string, c *call, by *peer) {
	l := h.line(name)
	c.timer.Stop()
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

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
