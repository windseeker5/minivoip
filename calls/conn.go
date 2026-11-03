package calls

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// outbox writes one socket's messages from its own goroutine, so a browser
// that stops reading can't hold up the hub, the rooms or anyone else's call.
// Past queueSize waiting messages it's cut off.
type outbox struct {
	conn  *websocket.Conn
	out   chan any
	done  chan struct{}
	once  sync.Once
	kills sync.Once
	last  atomic.Int64 // Unix seconds of the last message from the browser
}

const queueSize = 256

// closeAfter is queued to close the socket once what's before it was sent.
type closeAfter struct{}

func newOutbox(conn *websocket.Conn) *outbox {
	b := &outbox{conn: conn, out: make(chan any, queueSize), done: make(chan struct{})}
	b.touch()
	go b.run()
	return b
}

func (b *outbox) run() {
	for {
		select {
		case <-b.done:
			return
		case m := <-b.out:
			if _, last := m.(closeAfter); last {
				b.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := wsjson.Write(ctx, b.conn, m)
			cancel()
			if err != nil {
				log.Printf("calls: send: %v", err)
				b.conn.CloseNow()
				return
			}
		}
	}
}

// put queues a message; it never blocks.
func (b *outbox) put(m any) {
	select {
	case b.out <- m:
	default:
		log.Printf("calls: a browser stopped reading, closing its socket")
		b.kill()
	}
}

// kill closes the socket without waiting: closing can block for seconds while
// a close handshake is under way, and callers may hold the hub's lock.
func (b *outbox) kill() {
	b.kills.Do(func() { go b.conn.CloseNow() })
}

func (b *outbox) touch() { b.last.Store(time.Now().Unix()) }

func (b *outbox) idleFor() time.Duration {
	return time.Since(time.Unix(b.last.Load(), 0))
}

// stop ends the writer and closes the socket.
func (b *outbox) stop() {
	b.once.Do(func() { close(b.done) })
	b.kill()
}

// pace cuts off a socket sending far more than a real browser does: at most
// maxMessages in any paceWindow (a call's setup is a few dozen).
type pace struct {
	start time.Time
	n     int
}

const (
	maxMessages = 300
	paceWindow  = 10 * time.Second
)

func (p *pace) ok() bool {
	now := time.Now()
	if now.Sub(p.start) > paceWindow {
		p.start, p.n = now, 0
	}
	p.n++
	return p.n <= maxMessages
}
