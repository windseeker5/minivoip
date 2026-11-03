package calls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// roomServer is a room where the request with ?owner=1 is the owner.
func roomServer(t *testing.T) (*Rooms, *httptest.Server) {
	rs := NewRooms(func(string) bool { return true })
	rs.IsOwner = func(r *http.Request, line string) bool { return r.URL.Query().Get("owner") == "1" }
	srv := httptest.NewServer(rs)
	t.Cleanup(srv.Close)
	return rs, srv
}

func enter(t *testing.T, srv *httptest.Server, name string, owner bool) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?line=joesbnb"
	if owner {
		url += "&owner=1"
	}
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if err := wsjson.Write(context.Background(), c, RoomMessage{Type: "join", Name: name}); err != nil {
		t.Fatal(err)
	}
	return c
}

func roomExpect(t *testing.T, c *websocket.Conn, typ string) RoomMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var m RoomMessage
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatalf("waiting for %q: %v", typ, err)
	}
	if m.Type != typ {
		t.Fatalf("got %q, want %q", m.Type, typ)
	}
	return m
}

// door reads the owner's door list, the next message.
func door(t *testing.T, c *websocket.Conn) []RoomPeer {
	t.Helper()
	return roomExpect(t, c, "door").Peers
}

func TestRoomGuestsWaitUntilLetIn(t *testing.T) {
	rs, srv := roomServer(t)
	var mu sync.Mutex
	var knocks []string
	rs.OnKnock = func(line, name string) { mu.Lock(); knocks = append(knocks, name); mu.Unlock() }

	ana := enter(t, srv, "  Ana   Roy ", false)
	roomExpect(t, ana, "waiting")
	if w := rs.Waiting("joesbnb"); len(w) != 1 || w[0] != "Ana Roy" {
		t.Fatalf("waiting = %v", w)
	}

	// The owner opens the room: Ana still waits, and the owner sees her.
	owner := enter(t, srv, "Joe", true)
	if m := roomExpect(t, owner, "welcome"); len(m.Peers) != 0 || m.ID == "" {
		t.Fatalf("owner welcome = %+v", m)
	}
	list := door(t, owner)
	if len(list) != 1 || list[0].Name != "Ana Roy" {
		t.Fatalf("door = %+v", list)
	}
	if rs.People("joesbnb") != 1 {
		t.Fatalf("people %d: nobody comes in by themselves", rs.People("joesbnb"))
	}

	// A guest coming while the owner is in the room waits too.
	bo := enter(t, srv, "", false)
	roomExpect(t, bo, "waiting")
	list = door(t, owner)
	if len(list) != 2 || list[1].Name != "Guest" {
		t.Fatalf("door = %+v", list)
	}

	// A guest can't let anyone in, or send to anyone.
	wsjson.Write(context.Background(), bo, RoomMessage{Type: "admit", To: list[0].ID})
	wsjson.Write(context.Background(), bo, RoomMessage{Type: "admit", To: list[1].ID})
	wsjson.Write(context.Background(), bo, RoomMessage{Type: "signal", To: "x", Data: json.RawMessage(`{}`)})
	time.Sleep(50 * time.Millisecond)
	if rs.People("joesbnb") != 1 {
		t.Fatal("a waiting guest let someone in")
	}

	// The owner lets Ana in.
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "admit", To: list[0].ID})
	welcome := roomExpect(t, ana, "welcome")
	if len(welcome.Peers) != 1 || welcome.Peers[0].Name != "Joe" || !welcome.Peers[0].Owner {
		t.Fatalf("Ana's welcome = %+v", welcome)
	}
	joined := roomExpect(t, owner, "joined")
	if joined.Peer.Name != "Ana Roy" || joined.Peer.ID != welcome.ID {
		t.Fatalf("joined = %+v", joined.Peer)
	}
	if left := door(t, owner); len(left) != 1 {
		t.Fatalf("door = %+v", left)
	}

	// A guest in the room can't let anyone in either, or remove anyone.
	wsjson.Write(context.Background(), ana, RoomMessage{Type: "admit", To: list[1].ID})
	wsjson.Write(context.Background(), ana, RoomMessage{Type: "remove", To: list[0].ID})
	time.Sleep(50 * time.Millisecond)
	if rs.People("joesbnb") != 2 || len(rs.Waiting("joesbnb")) != 1 {
		t.Fatalf("people %d, waiting %v", rs.People("joesbnb"), rs.Waiting("joesbnb"))
	}

	// Messages go to the one person they're for, with who sent them.
	data := json.RawMessage(`{"type":"offer","sdp":"x"}`)
	wsjson.Write(context.Background(), ana, RoomMessage{Type: "signal", To: joined.Peer.ID, Data: data})
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "signal", To: welcome.ID, Data: data})
	if m := roomExpect(t, ana, "signal"); string(m.Data) != string(data) || m.From == "" {
		t.Fatalf("signal = %+v", m)
	}

	// Declining sends Bo away.
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "deny", To: list[1].ID})
	roomExpect(t, bo, "denied")
	if left := door(t, owner); len(left) != 0 {
		t.Fatalf("door = %+v", left)
	}

	// Removing sends Ana away, and tells the others.
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "remove", To: welcome.ID})
	roomExpect(t, ana, "removed")
	if m := roomExpect(t, owner, "left"); m.ID != welcome.ID {
		t.Fatalf("left = %+v", m)
	}
	if rs.People("joesbnb") != 1 {
		t.Fatalf("people %d", rs.People("joesbnb"))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(knocks) != 2 || knocks[0] != "Ana Roy" {
		t.Fatalf("knocks = %v", knocks)
	}
}

// A room keeps one seat for its owner, and says when it's full.
func TestRoomIsFull(t *testing.T) {
	rs, srv := roomServer(t)
	rs.Size = 2
	owner := enter(t, srv, "Joe", true)
	roomExpect(t, owner, "welcome")
	door(t, owner)
	ana := enter(t, srv, "Ana", false)
	roomExpect(t, ana, "waiting")
	first := door(t, owner)
	bo := enter(t, srv, "Bo", false)
	roomExpect(t, bo, "waiting")
	both := door(t, owner)
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "admit", To: first[0].ID})
	roomExpect(t, ana, "welcome")
	roomExpect(t, owner, "joined")
	door(t, owner)
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "admit", To: both[1].ID})
	roomExpect(t, owner, "seats-full")
	if w := rs.Waiting("joesbnb"); len(w) != 1 {
		t.Fatalf("Bo should still wait: %v", w)
	}
}

func TestRoomDoorHasALimit(t *testing.T) {
	_, srv := roomServer(t)
	for i := 0; i < maxWaiting; i++ {
		roomExpect(t, enter(t, srv, "Guest", false), "waiting")
	}
	roomExpect(t, enter(t, srv, "One too many", false), "full")
}

func TestRoomWaitingGuestLeaves(t *testing.T) {
	rs, srv := roomServer(t)
	ana := enter(t, srv, "Ana", false)
	roomExpect(t, ana, "waiting")
	ana.Close(websocket.StatusNormalClosure, "")
	for end := time.Now().Add(2 * time.Second); len(rs.Waiting("joesbnb")) > 0 && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	if w := rs.Waiting("joesbnb"); len(w) != 0 {
		t.Fatalf("still waiting: %v", w)
	}
}

// A socket flooding the room is cut off.
func TestRoomFlood(t *testing.T) {
	_, srv := roomServer(t)
	c := enter(t, srv, "Ana", false)
	roomExpect(t, c, "waiting")
	for i := 0; i < maxMessages+50; i++ {
		if err := wsjson.Write(context.Background(), c, RoomMessage{Type: "signal", To: "x"}); err != nil {
			return // closed already
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var m RoomMessage
	if err := wsjson.Read(ctx, c, &m); err == nil || ctx.Err() != nil {
		t.Fatalf("the socket stayed open: %+v %v", m, err)
	}
}

func TestRoomKnockLimit(t *testing.T) {
	rs, srv := roomServer(t)
	var mu sync.Mutex
	knocks := 0
	rs.OnKnock = func(line, name string) { mu.Lock(); knocks++; mu.Unlock() }
	rs.AllowKnock = func(r *http.Request, line string) bool { return false }

	// Past the limit the guest still waits, but the owner's phone stays quiet.
	ana := enter(t, srv, "Ana", false)
	roomExpect(t, ana, "waiting")
	if w := rs.Waiting("joesbnb"); len(w) != 1 {
		t.Fatalf("waiting = %v", w)
	}
	mu.Lock()
	defer mu.Unlock()
	if knocks != 0 {
		t.Fatalf("knocks = %d, want 0", knocks)
	}
}

// Someone at the door, one address: two may wait, and one who was declined
// waits a while to knock again. A login that ended can't act as the owner.
func TestRoomDoorLimits(t *testing.T) {
	rs, srv := roomServer(t)
	rs.GuestKey = func(r *http.Request) string { return r.URL.Query().Get("from") }
	join := func(from string) *websocket.Conn {
		c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/?line=joesbnb&from="+from, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.CloseNow() })
		wsjson.Write(context.Background(), c, RoomMessage{Type: "join", Name: "G"})
		return c
	}
	roomExpect(t, join("a"), "waiting")
	roomExpect(t, join("a"), "waiting")
	roomExpect(t, join("a"), "full")
	bo := join("b")
	roomExpect(t, bo, "waiting")
	owner := enter(t, srv, "Joe", true)
	roomExpect(t, owner, "welcome")
	var who string
	for _, p := range door(t, owner) {
		who = p.ID // the last one is Bo
	}
	wsjson.Write(context.Background(), owner, RoomMessage{Type: "deny", To: who})
	roomExpect(t, bo, "denied")
	roomExpect(t, join("b"), "denied")
}

func TestRoomWaitEnds(t *testing.T) {
	rs, srv := roomServer(t)
	rs.WaitLimit = 50 * time.Millisecond
	ana := enter(t, srv, "Ana", false)
	roomExpect(t, ana, "waiting")
	roomExpect(t, ana, "wait-over")
	if w := rs.Waiting("joesbnb"); len(w) != 0 {
		t.Fatalf("still waiting: %v", w)
	}
}

// A socket opened while logged in, joining after the login ended, is a guest.
func TestRoomOwnerLoginEnded(t *testing.T) {
	rs, srv := roomServer(t)
	var ended atomic.Bool
	rs.IsOwner = func(r *http.Request, line string) bool { return r.URL.Query().Get("owner") == "1" && !ended.Load() }
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/?line=joesbnb&owner=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	time.Sleep(50 * time.Millisecond)
	ended.Store(true)
	wsjson.Write(context.Background(), c, RoomMessage{Type: "join", Name: "Thief"})
	roomExpect(t, c, "waiting")
}

// A line that lost its room (moved to Basic) sends everyone away.
func TestRoomRecheckClosesRoomOfBasicLine(t *testing.T) {
	var basic atomic.Bool
	rs := NewRooms(func(string) bool { return !basic.Load() })
	rs.IsOwner = func(r *http.Request, line string) bool { return true }
	srv := httptest.NewServer(rs)
	defer srv.Close()
	owner := enter(t, srv, "Joe", true)
	roomExpect(t, owner, "welcome")
	door(t, owner)
	basic.Store(true)
	rs.RecheckAll()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var m RoomMessage
	if err := wsjson.Read(ctx, owner, &m); err == nil || ctx.Err() != nil {
		t.Fatalf("the socket stayed open: %+v", m)
	}
}
