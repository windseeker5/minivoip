package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestWindow(t *testing.T) {
	w := NewWindow(3, 50*time.Millisecond)
	for i := 0; i < 3; i++ {
		if !w.Take("a") {
			t.Fatalf("take %d refused", i+1)
		}
	}
	if w.Take("a") || !w.Full("a") {
		t.Fatal("the fourth is refused")
	}
	if w.Full("b") {
		t.Fatal("each key counts on its own")
	}
	time.Sleep(60 * time.Millisecond)
	if w.Full("a") || !w.Take("a") {
		t.Fatal("the window moves on")
	}
}

func TestClientIP(t *testing.T) {
	for _, c := range []struct {
		remote, xff string
		ip          string
		local       bool
	}{
		{"203.0.113.9:5000", "", "203.0.113.9", false},
		{"203.0.113.9:5000", "198.51.100.1", "203.0.113.9", false},           // not through our proxy: header ignored
		{"127.0.0.1:5000", "", "127.0.0.1", true},                            // this machine (tests, Ken)
		{"127.0.0.1:5000", "198.51.100.1", "198.51.100.1", false},            // Caddy
		{"127.0.0.1:5000", "198.51.100.1, 127.0.0.1", "198.51.100.1", false}, // dev proxy
		{"127.0.0.1:5000", "192.168.1.20", "127.0.0.1", true},                // home network
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if ip, local := clientIP(r); ip != c.ip || local != c.local {
			t.Errorf("%s %q: got %s %v, want %s %v", c.remote, c.xff, ip, local, c.ip, c.local)
		}
	}
}
