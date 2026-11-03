package web

import (
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
	ExemptHomeNetwork = true
	defer func() { ExemptHomeNetwork = false }()
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
		{"172.17.0.1:5000", "", "172.17.0.1", false},                         // a container's published port: not exempt
		{"172.17.0.1:5000", "10.0.0.5", "172.17.0.1", false},                 // and it can't claim to be local
		{"172.17.0.1:5000", "198.51.100.1", "172.17.0.1", false},             // or anyone else
		{"[2001:db8:1:2:3:4:5:6]:5000", "", "2001:db8:1:2::/64", false},      // IPv6 counts by network
		{"127.0.0.1:5000", "2001:db8:1:2:aa:bb:cc:dd", "2001:db8:1:2::/64", false},
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

// A container's proxy that names no visitor doesn't switch the limits off.
func TestProxyNamingNobodyIsNotLocal(t *testing.T) {
	TrustedProxies = []string{"172.20.0.0/16"}
	defer func() { TrustedProxies = nil }()
	for _, xff := range []string{"", "172.20.0.1", "172.20.0.1, 172.20.0.2"} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "172.20.0.2:5000"
		r.Header.Set("X-Forwarded-For", xff)
		if ip, local := clientIP(r); local || ip != "172.20.0.2" {
			t.Errorf("%q: got %s %v", xff, ip, local)
		}
	}
}

// Off (a container), a private address behind the proxy is just a visitor.
func TestHomeNetworkExemptionIsOptional(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "192.168.1.20")
	if ip, local := clientIP(r); ip != "192.168.1.20" || local {
		t.Errorf("got %s %v", ip, local)
	}
}

func TestTrustedProxyNames(t *testing.T) {
	TrustedProxies = []string{"172.20.0.5", " 10.9.0.0/16"}
	defer func() { TrustedProxies = nil }()
	for addr, want := range map[string]bool{"172.20.0.5": true, "10.9.3.4": true, "127.0.0.1": true, "172.20.0.6": false, "10.8.0.1": false, "nonsense": false} {
		if got := trustedProxy(addr); got != want {
			t.Errorf("%s: %v", addr, got)
		}
	}
}

func TestTakeIsOneStep(t *testing.T) {
	w := NewWindow(5, time.Minute)
	var got atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w.Take("a") {
				got.Add(1)
			}
		}()
	}
	wg.Wait()
	if got.Load() != 5 {
		t.Fatalf("%d passed", got.Load())
	}
	w.Give("a")
	if !w.Take("a") || w.Take("a") {
		t.Fatal("Give makes room for exactly one")
	}
}
