package calls

// Meeting rooms: each line has one, at /room/<line>. Like the call hub, the
// server only passes messages; voice, video, screens and the whiteboard go
// directly between the browsers, everyone connected to everyone (a mesh,
// fine for a few people). Guests wait until the owner is in the room.
//
// Browser -> server: join {name}, signal {to, data}
// Server -> browser: welcome {id, peers}, waiting, full, joined {peer}, left {id}, signal {from, data}
//
// A newcomer gets the people already in the room ("welcome") and offers a
// connection to each of them; they answer.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// RoomSize is how many people fit in a room: everyone sends their video to
// everyone else, so a few people only.
const RoomSize = 4

// RoomMessage is every message on a room's socket.
type RoomMessage struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	To    string          `json:"to,omitempty"`
	From  string          `json:"from,omitempty"`
	Peer  *RoomPeer       `json:"peer,omitempty"`
	Peers []RoomPeer      `json:"peers,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// RoomPeer is someone in a room.
type RoomPeer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner"`
}

type member struct {
	RoomPeer
	conn *websocket.Conn
	req  *http.Request // who connected, for AllowKnock
}

func (m *member) send(msg RoomMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, m.conn, msg); err != nil {
		log.Printf("rooms: send %s: %v", msg.Type, err)
	}
}

type room struct {
	in      []*member // in the room, in the order they came in
	waiting []*member // guests waiting for the owner
}

func (r *room) hasOwner() bool {
	for _, m := range r.in {
		if m.Owner {
			return true
		}
	}
	return false
}

// Rooms holds every line's meeting room.
type Rooms struct {
	// Size is how many people fit in one room (RoomSize when zero).
	Size int
	// OriginPatterns lists the hosts allowed to open a socket besides the
	// page's own host.
	OriginPatterns []string
	// LineExists says whether a line name is real and has a meeting room
	// (main.go: Premium lines only).
	LineExists func(name string) bool
	// IsOwner says whether this request (its login) is the line's owner,
	// who opens the room. Nil makes nobody the owner.
	IsOwner func(r *http.Request, line string) bool
	// OnWaiting gets the names of the guests waiting for the owner, every
	// time that list changes (empty when nobody waits). OnKnock says a new
	// guest started waiting. Both run with the rooms locked, so they must
	// not block. Nil skips them.
	OnWaiting func(line string, names []string)
	OnKnock   func(line, name string)
	// AllowKnock says whether this guest's knock may wake the owner's phone
	// (limits, web/limits.go); when not, the guest still waits. It runs
	// with the rooms locked. Nil allows every knock.
	AllowKnock func(r *http.Request, line string) bool

	mu    sync.Mutex
	rooms map[string]*room
}

func NewRooms(lineExists func(name string) bool) *Rooms {
	return &Rooms{LineExists: lineExists, rooms: map[string]*room{}}
}

func (rs *Rooms) room(name string) *room {
	r := rs.rooms[name]
	if r == nil {
		r = &room{}
		rs.rooms[name] = r
	}
	return r
}

func (rs *Rooms) size() int {
	if rs.Size > 0 {
		return rs.Size
	}
	return RoomSize
}

// Waiting lists the guests waiting in a line's room.
func (rs *Rooms) Waiting(line string) []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.rooms[line]
	if r == nil {
		return nil
	}
	return names(r.waiting)
}

// People says how many are in a line's room now.
func (rs *Rooms) People(line string) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if r := rs.rooms[line]; r != nil {
		return len(r.in)
	}
	return 0
}

func names(list []*member) []string {
	out := []string{}
	for _, m := range list {
		out = append(out, m.Name)
	}
	return out
}

// ServeHTTP is the socket at /ws/room?line=<name>.
func (rs *Rooms) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	line := r.URL.Query().Get("line")
	if !rs.LineExists(line) {
		http.Error(w, "unknown room", http.StatusBadRequest)
		return
	}
	owner := rs.IsOwner != nil && rs.IsOwner(r, line)
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: rs.OriginPatterns})
	if err != nil {
		return
	}
	conn.SetReadLimit(256 << 10) // offers with video are a few kB; keep it bounded
	me := &member{RoomPeer: RoomPeer{ID: newID(), Owner: owner}, conn: conn, req: r}
	defer rs.leave(line, me)
	for {
		var m RoomMessage
		if err := wsjson.Read(r.Context(), conn, &m); err != nil {
			return
		}
		rs.handle(line, me, m)
	}
}

func (rs *Rooms) handle(line string, me *member, m RoomMessage) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.room(line)
	switch m.Type {
	case "join":
		if me.Name != "" {
			return // joined already
		}
		me.Name = cleanName(m.Name)
		if !me.Owner && !r.hasOwner() {
			r.waiting = append(r.waiting, me)
			log.Printf("rooms: %s: %s is waiting", line, me.Name)
			me.send(RoomMessage{Type: "waiting"})
			if rs.OnKnock != nil && (rs.AllowKnock == nil || rs.AllowKnock(me.req, line)) {
				rs.OnKnock(line, me.Name)
			}
			rs.waitingChanged(line, r)
			return
		}
		rs.admit(line, r, me)
		if me.Owner && len(r.waiting) > 0 {
			// The owner opens the room: the guests waiting come in.
			list := r.waiting
			r.waiting = nil
			for _, g := range list {
				rs.admit(line, r, g)
			}
			rs.waitingChanged(line, r)
		}

	case "signal":
		if !in(r.in, me) {
			return
		}
		for _, o := range r.in {
			if o.ID == m.To && o != me {
				o.send(RoomMessage{Type: "signal", From: me.ID, Data: m.Data})
			}
		}
	}
}

// admit lets someone in, or says the room is full. Caller holds rs.mu.
func (rs *Rooms) admit(line string, r *room, me *member) {
	if len(r.in) >= rs.size() {
		me.send(RoomMessage{Type: "full"})
		return
	}
	peers := []RoomPeer{}
	for _, o := range r.in {
		peers = append(peers, o.RoomPeer)
	}
	r.in = append(r.in, me)
	log.Printf("rooms: %s: %s came in (%d in the room)", line, me.Name, len(r.in))
	me.send(RoomMessage{Type: "welcome", ID: me.ID, Peers: peers})
	p := me.RoomPeer
	for _, o := range r.in {
		if o != me {
			o.send(RoomMessage{Type: "joined", Peer: &p})
		}
	}
}

// leave runs when a socket closes.
func (rs *Rooms) leave(line string, me *member) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.room(line)
	if in(r.waiting, me) {
		r.waiting = without(r.waiting, me)
		rs.waitingChanged(line, r)
	}
	if in(r.in, me) {
		r.in = without(r.in, me)
		log.Printf("rooms: %s: %s left (%d in the room)", line, me.Name, len(r.in))
		for _, o := range r.in {
			o.send(RoomMessage{Type: "left", ID: me.ID})
		}
	}
	if len(r.in) == 0 && len(r.waiting) == 0 {
		delete(rs.rooms, line)
	}
	me.conn.CloseNow()
}

func (rs *Rooms) waitingChanged(line string, r *room) {
	if rs.OnWaiting != nil {
		rs.OnWaiting(line, names(r.waiting))
	}
}

func in(list []*member, m *member) bool {
	for _, x := range list {
		if x == m {
			return true
		}
	}
	return false
}

func without(list []*member, m *member) []*member {
	out := list[:0:0]
	for _, x := range list {
		if x != m {
			out = append(out, x)
		}
	}
	return out
}

// cleanName keeps a shown name short and on one line.
func cleanName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Guest"
	}
	if utf8.RuneCountInString(s) > 40 {
		s = string([]rune(s)[:40])
	}
	return s
}
