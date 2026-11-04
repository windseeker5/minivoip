package calls

// Meeting rooms: each line has one, at /room/<line>. Like the call hub, the
// server only passes messages; voice, video, screens and the whiteboard go
// directly between the browsers, everyone connected to everyone (a mesh,
// fine for a few people). Guests wait until the owner lets them in, one by
// one: knowing the room's address isn't enough to hear a meeting.
//
// Browser -> server: join {name}, signal {to, data}; the owner also admit {to},
// deny {to}, remove {to}
// Server -> browser: welcome {id, peers}, waiting, full, joined {peer}, left {id}, signal {from, data},
// denied, removed, wait-over; to the owner door {peers: who waits}, seats-full
//
// A newcomer gets the people already in the room ("welcome") and offers a
// connection to each of them; they answer.

import (
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
// everyone else, so a few people only. One seat is the owner's.
const RoomSize = 4

// A door keeps out robots: at most 10 guests wait, at most 2 of them from one
// address, for 10 minutes. A socket that hasn't said who it is after a minute
// is closed, and someone declined or removed waits 5 minutes to knock again.
const (
	maxWaiting      = 10
	maxWaitingPerIP = 2
	joinLimit       = time.Minute
	waitLimit       = 10 * time.Minute
	knockBan        = 5 * time.Minute
)

// RoomMessage is every message on a room's socket.
type RoomMessage struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Told  bool            `json:"told,omitempty"` // "join": this guest's page told them notes are on
	To    string          `json:"to,omitempty"`
	From  string          `json:"from,omitempty"`
	Peer  *RoomPeer       `json:"peer,omitempty"`
	Peers []RoomPeer      `json:"peers,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	ICE   json.RawMessage `json:"ice,omitempty"` // "welcome": the connection helpers, with a relay login
}

// RoomPeer is someone in a room.
type RoomPeer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner"`
	Told  bool   `json:"told,omitempty"` // their page told them notes are on
}

type member struct {
	RoomPeer
	*outbox
	req *http.Request // who connected, for AllowKnock
	key string        // the guest's address for the door's limits; "" isn't limited
}

func (m *member) send(msg RoomMessage) { m.put(msg) }

type room struct {
	in      []*member // in the room, in the order they came in
	waiting []*member // guests waiting for the owner to let them in
}

func (r *room) guests() int {
	n := 0
	for _, m := range r.in {
		if !m.Owner {
			n++
		}
	}
	return n
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
	// OnJoined says a guest came into the room. It runs with the rooms locked,
	// so it must not block. Nil skips it.
	OnJoined func(line, name string)
	// AllowKnock says whether this guest's knock may wake the owner's phone
	// (limits, web/limits.go); when not, the guest still waits. It runs
	// with the rooms locked. Nil allows every knock.
	AllowKnock func(r *http.Request, line string) bool
	// GuestKey gives the address a guest counts under at the door; "" isn't
	// limited (this machine). Nil limits nobody.
	GuestKey func(r *http.Request) string
	// ICE gives someone let in the connection helpers, with a relay login. It
	// runs with the rooms locked. Nil sends none.
	ICE func() json.RawMessage
	// WaitLimit is how long a guest waits at the door (waitLimit when zero).
	WaitLimit time.Duration

	mu     sync.Mutex
	rooms  map[string]*room
	banned map[string]time.Time // line + key -> no knocking until then
}

func NewRooms(lineExists func(name string) bool) *Rooms {
	return &Rooms{LineExists: lineExists, rooms: map[string]*room{}, banned: map[string]time.Time{}}
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

// Meeting lists the guests in a line's room while its owner is there with them;
// on is false when no meeting is on, told when one of them wasn't told about notes.
func (rs *Rooms) Meeting(line string) (guests []string, on, told bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.rooms[line]
	if r == nil {
		return nil, false, false
	}
	host := false
	told = true
	for _, m := range r.in {
		if m.Owner {
			host = true
		} else {
			guests = append(guests, m.Name)
			told = told && m.Told
		}
	}
	return guests, host && len(guests) > 0, told
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
	conn.SetReadLimit(64 << 10) // offers with video are a few kB; keep it bounded
	me := &member{RoomPeer: RoomPeer{ID: newID(), Owner: owner}, outbox: newOutbox(conn), req: r}
	if rs.GuestKey != nil {
		me.key = rs.GuestKey(r)
	}
	defer rs.leave(line, me)
	// A socket that never says who it is goes away.
	silent := time.AfterFunc(joinLimit, func() {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		if me.Name == "" {
			me.kill()
		}
	})
	defer silent.Stop()
	var speed pace
	for {
		var m RoomMessage
		if err := wsjson.Read(r.Context(), conn, &m); err != nil {
			return
		}
		if !speed.ok() {
			log.Printf("rooms: %s: too many messages from one socket, closing it", line)
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
		me.Told = m.Told
		// The login may have ended since this socket opened.
		me.Owner = me.Owner && rs.IsOwner != nil && rs.IsOwner(me.req, line)
		if !me.Owner {
			if rs.isBanned(line, me.key) {
				me.send(RoomMessage{Type: "denied"})
				me.put(closeAfter{})
				return
			}
			if len(r.waiting) >= maxWaiting || me.key != "" && waitingFrom(r.waiting, me.key) >= maxWaitingPerIP {
				me.send(RoomMessage{Type: "full"})
				me.put(closeAfter{})
				return
			}
			r.waiting = append(r.waiting, me)
			rs.expireWait(line, me)
			log.Printf("rooms: %s: a guest is waiting", line)
			me.send(RoomMessage{Type: "waiting"})
			if rs.OnKnock != nil && (rs.AllowKnock == nil || rs.AllowKnock(me.req, line)) {
				rs.OnKnock(line, me.Name)
			}
			rs.waitingChanged(line, r)
			return
		}
		rs.admit(line, r, me)
		me.send(RoomMessage{Type: "door", Peers: peersOf(r.waiting)})

	case "admit", "deny":
		if !me.Owner || !in(r.in, me) {
			return
		}
		g := find(r.waiting, m.To)
		if g == nil {
			return
		}
		if m.Type == "admit" && r.guests() >= rs.size()-1 {
			me.send(RoomMessage{Type: "seats-full"})
			return
		}
		r.waiting = without(r.waiting, g)
		if m.Type == "admit" {
			rs.admit(line, r, g)
		} else {
			rs.ban(line, g.key)
			g.send(RoomMessage{Type: "denied"})
			g.put(closeAfter{})
		}
		rs.waitingChanged(line, r)

	case "remove":
		if !me.Owner || !in(r.in, me) {
			return
		}
		g := find(r.in, m.To)
		if g == nil || g.Owner {
			return
		}
		rs.drop(line, r, g)
		rs.ban(line, g.key)
		g.send(RoomMessage{Type: "removed"})
		g.put(closeAfter{})

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
	if len(r.in) >= rs.size() || !me.Owner && r.guests() >= rs.size()-1 {
		me.send(RoomMessage{Type: "full"})
		return
	}
	peers := []RoomPeer{}
	for _, o := range r.in {
		peers = append(peers, o.RoomPeer)
	}
	r.in = append(r.in, me)
	log.Printf("rooms: %s: someone came in (%d in the room)", line, len(r.in))
	welcome := RoomMessage{Type: "welcome", ID: me.ID, Peers: peers}
	if rs.ICE != nil {
		welcome.ICE = rs.ICE()
	}
	me.send(welcome)
	p := me.RoomPeer
	for _, o := range r.in {
		if o != me {
			o.send(RoomMessage{Type: "joined", Peer: &p})
		}
	}
	if !me.Owner && rs.OnJoined != nil {
		rs.OnJoined(line, me.Name)
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
	rs.drop(line, r, me)
	if len(r.in) == 0 && len(r.waiting) == 0 {
		delete(rs.rooms, line)
	}
	me.stop()
}

// drop takes someone out of the room and tells the others. Caller holds rs.mu.
func (rs *Rooms) drop(line string, r *room, me *member) {
	if !in(r.in, me) {
		return
	}
	r.in = without(r.in, me)
	log.Printf("rooms: %s: someone left (%d in the room)", line, len(r.in))
	for _, o := range r.in {
		o.send(RoomMessage{Type: "left", ID: me.ID})
	}
}

// Recheck sends away an owner whose login ended (logged out, or the password
// changed): they'd keep letting people in otherwise. A line that has no room
// any more (moved to Basic) sends everyone away.
func (rs *Rooms) Recheck(line string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r := rs.rooms[line]
	if r == nil {
		return
	}
	gone := !rs.LineExists(line)
	for _, m := range r.in {
		if gone || m.Owner && (rs.IsOwner == nil || !rs.IsOwner(m.req, line)) {
			m.kill()
		}
	}
	if gone {
		for _, m := range r.waiting {
			m.kill()
		}
	}
}

// RecheckAll runs Recheck on every open room, for logins that ran out and
// plans that changed.
func (rs *Rooms) RecheckAll() {
	rs.mu.Lock()
	names := make([]string, 0, len(rs.rooms))
	for name := range rs.rooms {
		names = append(names, name)
	}
	now := time.Now()
	for k, until := range rs.banned {
		if now.After(until) {
			delete(rs.banned, k)
		}
	}
	rs.mu.Unlock()
	for _, name := range names {
		rs.Recheck(name)
	}
}

// ban keeps someone declined or removed from knocking again for a while.
func (rs *Rooms) ban(line, key string) {
	if key != "" {
		rs.banned[line+" "+key] = time.Now().Add(knockBan)
	}
}

func (rs *Rooms) isBanned(line, key string) bool {
	return key != "" && time.Now().Before(rs.banned[line+" "+key])
}

// expireWait sends a guest away if nobody answered in time.
func (rs *Rooms) expireWait(line string, me *member) {
	limit := rs.WaitLimit
	if limit == 0 {
		limit = waitLimit
	}
	time.AfterFunc(limit, func() {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		r := rs.rooms[line]
		if r == nil || !in(r.waiting, me) {
			return
		}
		r.waiting = without(r.waiting, me)
		me.send(RoomMessage{Type: "wait-over"})
		me.put(closeAfter{})
		rs.waitingChanged(line, r)
	})
}

func waitingFrom(list []*member, key string) int {
	n := 0
	for _, m := range list {
		if m.key == key {
			n++
		}
	}
	return n
}

// waitingChanged tells the owner, in the room and on their apps, who waits.
func (rs *Rooms) waitingChanged(line string, r *room) {
	if rs.OnWaiting != nil {
		rs.OnWaiting(line, names(r.waiting))
	}
	for _, m := range r.in {
		if m.Owner {
			m.send(RoomMessage{Type: "door", Peers: peersOf(r.waiting)})
		}
	}
}

func peersOf(list []*member) []RoomPeer {
	out := []RoomPeer{}
	for _, m := range list {
		out = append(out, m.RoomPeer)
	}
	return out
}

func find(list []*member, id string) *member {
	for _, m := range list {
		if m.ID == id {
			return m
		}
	}
	return nil
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
