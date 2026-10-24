package calls

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func dial(t *testing.T, srv *httptest.Server, role string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?line=joesbnb&role=" + role
	c, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

// ready connects a phone app and waits until the hub has it.
func ready(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	c := dial(t, srv, "owner")
	send(t, c, Message{Type: "hello"})
	expect(t, c, "ready")
	return c
}

func send(t *testing.T, c *websocket.Conn, m Message) {
	t.Helper()
	if err := wsjson.Write(context.Background(), c, m); err != nil {
		t.Fatal(err)
	}
}

func expect(t *testing.T, c *websocket.Conn, typ string) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var m Message
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatalf("waiting for %q: %v", typ, err)
	}
	if m.Type != typ {
		t.Fatalf("got %q, want %q", m.Type, typ)
	}
	return m
}

func TestAnsweredCall(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	answered := make(chan string, 1)
	h.OnAnswered = func(line, callID string) { answered <- callID }
	srv := httptest.NewServer(h)
	defer srv.Close()
	owner := ready(t, srv)
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call", Option: "Talk to Joe"})
	id := expect(t, visitor, "ringing").ID
	if m := expect(t, owner, "incoming"); m.ID != id || m.Option != "Talk to Joe" {
		t.Fatalf("incoming = %+v", m)
	}

	send(t, owner, Message{Type: "accept", ID: id})
	expect(t, visitor, "accepted")
	if got := <-answered; got != id {
		t.Fatalf("OnAnswered got %q, want %q", got, id)
	}

	send(t, visitor, Message{Type: "offer", ID: id, SDP: "v=0 offer"})
	if m := expect(t, owner, "offer"); m.SDP != "v=0 offer" {
		t.Fatalf("offer = %+v", m)
	}
	send(t, owner, Message{Type: "answer", ID: id, SDP: "v=0 answer"})
	expect(t, visitor, "answer")

	send(t, visitor, Message{Type: "hangup", ID: id})
	expect(t, owner, "hangup")
}

func TestNoAnswer(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	h.RingTimeout = 100 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()
	owner := ready(t, srv)
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call"})
	expect(t, visitor, "ringing")
	expect(t, owner, "incoming")
	token := expect(t, visitor, "no-answer").Token
	expect(t, owner, "missed")

	if p, ok := h.UseVoicemailToken(token); !ok || p.Line != "joesbnb" || p.CallID == "" {
		t.Fatalf("voicemail token: %+v %v", p, ok)
	}
	if _, ok := h.UseVoicemailToken(token); ok {
		t.Fatal("a voicemail token works only once")
	}
}

func TestVoicemailChoice(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	srv := httptest.NewServer(h)
	defer srv.Close()
	visitor := dial(t, srv, "visitor")
	send(t, visitor, Message{Type: "voicemail", Option: "Leave a message"})
	token := expect(t, visitor, "voicemail").Token
	if p, ok := h.UseVoicemailToken(token); !ok || p.Option != "Leave a message" || p.CallID != "" {
		t.Fatalf("token: %+v %v", p, ok)
	}
}

func TestBusy(t *testing.T) {
	srv := httptest.NewServer(NewHub(func(string) bool { return true }))
	defer srv.Close()
	first := dial(t, srv, "visitor")
	send(t, first, Message{Type: "call"})
	expect(t, first, "ringing")

	second := dial(t, srv, "visitor")
	send(t, second, Message{Type: "call"})
	expect(t, second, "busy")
}

// records collects the hub's finished calls.
func records(h *Hub) chan Record {
	ch := make(chan Record, 4)
	h.OnEnd = func(line string, r Record) { ch <- r }
	return ch
}

func TestCallHistory(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	ended := records(h)
	srv := httptest.NewServer(h)
	defer srv.Close()
	owner := ready(t, srv)
	visitor := dial(t, srv, "visitor")

	// Answered, then hung up.
	send(t, visitor, Message{Type: "call", Option: "Book a room"})
	id := expect(t, visitor, "ringing").ID
	expect(t, owner, "incoming")
	send(t, owner, Message{Type: "accept", ID: id})
	expect(t, visitor, "accepted")
	send(t, owner, Message{Type: "hangup", ID: id})
	expect(t, visitor, "hangup")
	r := <-ended
	if r.ID != id || r.Outcome != "answered" || r.Option != "Book a room" || r.Answered.IsZero() || r.Ended.Before(r.Answered) {
		t.Fatalf("answered call = %+v", r)
	}

	// Declined: the caller is offered voicemail for that call.
	send(t, visitor, Message{Type: "call"})
	id = expect(t, visitor, "ringing").ID
	expect(t, owner, "incoming")
	send(t, owner, Message{Type: "decline", ID: id})
	token := expect(t, visitor, "no-answer").Token
	expect(t, owner, "missed")
	if r := <-ended; r.Outcome != "declined" || !r.Answered.IsZero() {
		t.Fatalf("declined call = %+v", r)
	}
	if p, _ := h.UseVoicemailToken(token); p.CallID != id {
		t.Fatalf("voicemail pass = %+v, want call %s", p, id)
	}

	// The caller hangs up while it rings.
	send(t, visitor, Message{Type: "call"})
	id = expect(t, visitor, "ringing").ID
	expect(t, owner, "incoming")
	send(t, visitor, Message{Type: "hangup", ID: id})
	if r := <-ended; r.Outcome != "missed" {
		t.Fatalf("hung-up call = %+v", r)
	}
}

func TestAway(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	h.Available = func(string) bool { return false }
	ended := records(h)
	srv := httptest.NewServer(h)
	defer srv.Close()
	owner := ready(t, srv)
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call", Option: "Talk to us"})
	m := expect(t, visitor, "no-answer")
	if r := <-ended; r.Outcome != "away" || r.ID != m.ID {
		t.Fatalf("away call = %+v", r)
	}
	if p, ok := h.UseVoicemailToken(m.Token); !ok || p.CallID != m.ID {
		t.Fatalf("away voicemail pass = %+v %v", p, ok)
	}
	// The phone app never rang.
	send(t, owner, Message{Type: "hello"})
	expect(t, owner, "ready")
}

// The Android app's ring screen declines over HTTP, not the socket.
func TestDeclineFromOutside(t *testing.T) {
	h := NewHub(func(string) bool { return true })
	srv := httptest.NewServer(h)
	defer srv.Close()
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call"})
	id := expect(t, visitor, "ringing").ID
	if h.Decline("joesbnb", "someothercall") {
		t.Fatal("declined a call that isn't ringing")
	}
	if !h.Decline("joesbnb", id) {
		t.Fatal("Decline didn't find the ringing call")
	}
	if m := expect(t, visitor, "no-answer"); m.Token == "" {
		t.Fatal("a declined caller should be offered voicemail")
	}
	if h.Decline("joesbnb", id) {
		t.Fatal("declined twice")
	}
}
