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
	srv := httptest.NewServer(NewHub())
	defer srv.Close()
	owner := dial(t, srv, "owner")
	send(t, owner, Message{Type: "hello"})
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call", Option: "Talk to Joe"})
	id := expect(t, visitor, "ringing").ID
	if m := expect(t, owner, "incoming"); m.ID != id || m.Option != "Talk to Joe" {
		t.Fatalf("incoming = %+v", m)
	}

	send(t, owner, Message{Type: "accept", ID: id})
	expect(t, visitor, "accepted")

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
	h := NewHub()
	h.RingTimeout = 100 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()
	owner := dial(t, srv, "owner")
	visitor := dial(t, srv, "visitor")

	send(t, visitor, Message{Type: "call"})
	expect(t, visitor, "ringing")
	expect(t, owner, "incoming")
	expect(t, visitor, "no-answer")
	expect(t, owner, "missed")
}

func TestBusy(t *testing.T) {
	srv := httptest.NewServer(NewHub())
	defer srv.Close()
	first := dial(t, srv, "visitor")
	send(t, first, Message{Type: "call"})
	expect(t, first, "ringing")

	second := dial(t, srv, "visitor")
	send(t, second, Message{Type: "call"})
	expect(t, second, "busy")
}
