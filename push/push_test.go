package push

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"minivoip/store"
)

// fakeService stands in for a browser's push service (FCM, Apple...).
func fakeService(t *testing.T, status int, got *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") ||
			r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("not a web push request: %s %v", r.Method, r.Header)
		}
		got.Add(1)
		w.WriteHeader(status)
	}))
}

// setup makes a store with Joe's line and one device pointing at endpoint.
func setup(t *testing.T, endpoint string) (*store.Store, *Sender) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	u, _ := st.CreateAccount("joe@example.com", "password1", "")
	st.CreateLine(u.AccountID, "joesbnb", "Joe's B&B")

	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	b64 := base64.RawURLEncoding.EncodeToString
	st.SaveDevice(u.AccountID, store.Device{Endpoint: endpoint, P256dh: b64(key.PublicKey().Bytes()), Auth: b64(auth)})

	s, err := New(st, "mailto:test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return st, s
}

func TestKeysAreKept(t *testing.T) {
	st, s := setup(t, "https://push.example/x")
	again, _ := New(st, "mailto:test@example.com")
	if s.PublicKey() == "" || again.PublicKey() != s.PublicKey() {
		t.Fatal("the VAPID keys should be made once and reused")
	}
}

func TestToLine(t *testing.T) {
	var got atomic.Int32
	svc := fakeService(t, http.StatusCreated, &got)
	defer svc.Close()
	_, s := setup(t, svc.URL+"/sub/1")
	if n := s.ToLine("joesbnb", Notification{Kind: "test", Title: "Hi"}, time.Minute, true); n != 1 || got.Load() != 1 {
		t.Fatalf("delivered %d, service got %d", n, got.Load())
	}
}

func TestGoneDeviceIsForgotten(t *testing.T) {
	var got atomic.Int32
	svc := fakeService(t, http.StatusGone, &got)
	defer svc.Close()
	st, s := setup(t, svc.URL+"/sub/1")
	s.ToLine("joesbnb", Notification{Kind: "test"}, time.Minute, false)
	if d, _ := st.DevicesForLine("joesbnb"); len(d) != 0 {
		t.Fatal("a 410 Gone subscription should be deleted")
	}
}

func TestRingRepeatsUntilStopped(t *testing.T) {
	RingEvery, RingFor = 50*time.Millisecond, 5*time.Second
	defer func() { RingEvery, RingFor = 5*time.Second, 30*time.Second }()
	var got atomic.Int32
	svc := fakeService(t, http.StatusCreated, &got)
	defer svc.Close()
	_, s := setup(t, svc.URL+"/sub/1")

	stop := s.Ring("joesbnb", "abc", "Talk to Joe")
	time.Sleep(180 * time.Millisecond)
	stop()
	stop() // safe twice
	n := got.Load()
	if n < 3 {
		t.Fatalf("expected repeated ringing, got %d pushes", n)
	}
	time.Sleep(150 * time.Millisecond)
	if got.Load() > n+1 {
		t.Fatalf("ringing kept going after stop: %d then %d", n, got.Load())
	}
}
