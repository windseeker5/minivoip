package calls

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestRoomGuestsWaitForTheOwner(t *testing.T) {
	rs, srv := roomServer(t)
	var mu sync.Mutex
	var knocks []string
	var waiting [][]string
	rs.OnKnock = func(line, name string) { mu.Lock(); knocks = append(knocks, name); mu.Unlock() }
	rs.OnWaiting = func(line string, names []string) { mu.Lock(); waiting = append(waiting, names); mu.Unlock() }

	ana := enter(t, srv, "  Ana   Roy ", false)
	roomExpect(t, ana, "waiting")
	if w := rs.Waiting("joesbnb"); len(w) != 1 || w[0] != "Ana Roy" {
		t.Fatalf("waiting = %v", w)
	}

	// The owner opens the room: Ana comes in and knows the owner is there.
	owner := enter(t, srv, "Joe", true)
	if m := roomExpect(t, owner, "welcome"); len(m.Peers) != 0 || m.ID == "" {
		t.Fatalf("owner welcome = %+v", m)
	}
	welcome := roomExpect(t, ana, "welcome")
	if len(welcome.Peers) != 1 || welcome.Peers[0].Name != "Joe" || !welcome.Peers[0].Owner {
		t.Fatalf("Ana's welcome = %+v", welcome)
	}
	joined := roomExpect(t, owner, "joined")
	if joined.Peer.Name != "Ana Roy" || joined.Peer.ID != welcome.ID {
		t.Fatalf("joined = %+v", joined.Peer)
	}
	if rs.People("joesbnb") != 2 || len(rs.Waiting("joesbnb")) != 0 {
		t.Fatalf("people %d, waiting %v", rs.People("joesbnb"), rs.Waiting("joesbnb"))
	}

	// A guest coming now goes straight in, with both already there.
	bo := enter(t, srv, "", false)
	if m := roomExpect(t, bo, "welcome"); len(m.Peers) != 2 {
		t.Fatalf("Bo's welcome = %+v", m)
	}
	if p := roomExpect(t, owner, "joined").Peer; p.Name != "Guest" {
		t.Fatalf("a guest without a name is %q", p.Name)
	}
	roomExpect(t, ana, "joined")

	// Messages go to the one person they're for, with who sent them.
	data := json.RawMessage(`{"type":"offer","sdp":"x"}`)
	wsjson.Write(context.Background(), bo, RoomMessage{Type: "signal", To: welcome.ID, Data: data})
	if m := roomExpect(t, ana, "signal"); string(m.Data) != string(data) || m.From == "" {
		t.Fatalf("signal = %+v", m)
	}

	// Leaving tells the others.
	bo.Close(websocket.StatusNormalClosure, "")
	roomExpect(t, owner, "left")
	roomExpect(t, ana, "left")

	mu.Lock()
	defer mu.Unlock()
	if len(knocks) != 1 || knocks[0] != "Ana Roy" {
		t.Fatalf("knocks = %v", knocks)
	}
	if len(waiting) != 2 || len(waiting[0]) != 1 || len(waiting[1]) != 0 {
		t.Fatalf("waiting changes = %v", waiting)
	}
}

func TestRoomIsFull(t *testing.T) {
	rs, srv := roomServer(t)
	rs.Size = 2
	owner := enter(t, srv, "Joe", true)
	roomExpect(t, owner, "welcome")
	ana := enter(t, srv, "Ana", false)
	roomExpect(t, ana, "welcome")
	roomExpect(t, enter(t, srv, "Bo", false), "full")
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
	// A guest can't send to anyone before coming in.
	bo := enter(t, srv, "Bo", false)
	roomExpect(t, bo, "waiting")
	wsjson.Write(context.Background(), bo, RoomMessage{Type: "signal", To: "x", Data: json.RawMessage(`{}`)})
	owner := enter(t, srv, "Joe", true)
	roomExpect(t, owner, "welcome")
	roomExpect(t, owner, "joined")
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
