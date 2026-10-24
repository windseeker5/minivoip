// Package push wakes the owner's phone: web push notifications (the free
// VAPID standard) for the browser app, and Firebase (fcm.go) for the
// Android app, which can ring like a phone call. The VAPID keys are made on
// first start and kept in the database's setting table.
package push

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"minivoip/store"
)

// Notification is the JSON the service worker (assets/js/sw.js) shows.
type Notification struct {
	Kind     string `json:"kind"` // ring, answered, missed, voicemail, test
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	Tag      string `json:"tag"`      // same tag replaces the previous notification
	Renotify bool   `json:"renotify"` // sound and vibrate again when replacing
	ID       int64  `json:"id"`       // push_log row; the phone reports it back
	Call     string `json:"call,omitempty"`
	// Until (Unix ms) is when a ring stops. A ring that arrives later is
	// shown as a missed call instead.
	Until int64 `json:"until,omitempty"`
}

// RingEvery and RingFor shape the "ringing": a fresh notification every few
// seconds until the call is answered or stops ringing. Faster repeats make
// Android quiet the app down ("notification cooldown").
var (
	RingEvery = 10 * time.Second
	RingFor   = 30 * time.Second
)

type Sender struct {
	Store   *store.Store
	Subject string // "mailto:..." or https URL, sent to the push services
	// Client is used for tests; nil means the default HTTP client.
	Client webpush.HTTPClient

	public, private string
	fcm             *fcm
}

// New loads the VAPID keys, making them the first time.
func New(st *store.Store, subject string) (*Sender, error) {
	s := &Sender{Store: st, Subject: subject, fcm: &fcm{dir: FirebaseDir(), client: &http.Client{Timeout: 15 * time.Second}}}
	var err error
	if s.public, err = st.Setting("vapid_public"); err != nil {
		return nil, err
	}
	if s.private, err = st.Setting("vapid_private"); err != nil {
		return nil, err
	}
	if s.public == "" || s.private == "" {
		if s.private, s.public, err = webpush.GenerateVAPIDKeys(); err != nil {
			return nil, err
		}
		if err := st.SetSetting("vapid_private", s.private); err != nil {
			return nil, err
		}
		if err := st.SetSetting("vapid_public", s.public); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// PublicKey goes to the browser to subscribe (applicationServerKey).
func (s *Sender) PublicKey() string { return s.public }

// Send delivers one notification to one device. A device the push service
// says is gone is forgotten.
func (s *Sender) Send(d store.Device, n Notification, ttl time.Duration, urgent bool) error {
	if token, ok := strings.CutPrefix(d.Endpoint, FCMPrefix); ok {
		err := s.fcm.send(token, n, ttl, urgent)
		if err == errGone {
			s.Store.DeleteDevice(d.Endpoint)
		}
		return err
	}
	body, _ := json.Marshal(n)
	urgency := webpush.UrgencyNormal
	if urgent {
		urgency = webpush.UrgencyHigh
	}
	res, err := webpush.SendNotification(body, &webpush.Subscription{
		Endpoint: d.Endpoint,
		Keys:     webpush.Keys{P256dh: d.P256dh, Auth: d.Auth},
	}, &webpush.Options{
		HTTPClient:      s.Client,
		Subscriber:      s.Subject,
		VAPIDPublicKey:  s.public,
		VAPIDPrivateKey: s.private,
		TTL:             int(ttl.Seconds()),
		Urgency:         urgency,
		Topic:           topic(n.Tag),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusGone || res.StatusCode == http.StatusNotFound {
		s.Store.DeleteDevice(d.Endpoint)
		return errGone
	}
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return &statusError{res.StatusCode, string(bytes.TrimSpace(msg))}
	}
	return nil
}

// topic lets the push service replace a notification still waiting for a
// sleeping phone with a newer one of the same tag, so a phone that wakes up
// gets one "Missed call" instead of a pile of rings. Topics are at most 32
// URL-safe base64 characters.
func topic(tag string) string {
	if len(tag) > 32 {
		return ""
	}
	for _, r := range tag {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return ""
		}
	}
	return tag
}

// ToLine sends a notification to every device of the line's owner and
// returns how many accepted it. Each send is written to the push log.
func (s *Sender) ToLine(line string, n Notification, ttl time.Duration, urgent bool) int {
	devices, err := s.Store.DevicesForLine(line)
	if err != nil {
		log.Printf("push: devices for %s: %v", line, err)
		return 0
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for _, d := range devices {
		wg.Add(1)
		go func(d store.Device, n Notification) {
			defer wg.Done()
			var err error
			if n.ID, err = s.Store.LogPush(d.ID, line, n.Call, n.Kind); err != nil {
				log.Printf("push: log: %v", err)
			}
			err = s.Send(d, n, ttl, urgent)
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			if n.ID != 0 {
				s.Store.PushResult(n.ID, result)
			}
			if err != nil {
				log.Printf("push: %s device %d: %v", line, d.ID, err)
				return
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}(d, n)
	}
	wg.Wait()
	return ok
}

// Ring notifies the owner's devices now and every RingEvery until stop is
// called or RingFor has passed. It returns at once; stop is safe to call
// more than once.
func (s *Sender) Ring(line, callID, option string) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	body := "A visitor is calling from your website."
	if option != "" {
		body = "A visitor pressed " + option + "."
	}
	n := Notification{Kind: "ring", Title: "Incoming call", Body: body, URL: "/app", Tag: "call-" + callID, Renotify: true,
		Call: callID, Until: time.Now().Add(RingFor).UnixMilli()}
	go func() {
		deadline := time.After(RingFor)
		tick := time.NewTicker(RingEvery)
		defer tick.Stop()
		for {
			// A ring may wait for a sleeping phone until the call is over;
			// the phone shows a late one as a missed call.
			s.ToLine(line, n, RingFor+30*time.Second, true)
			select {
			case <-done:
				return
			case <-deadline:
				return
			case <-tick.C:
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// Missed replaces the ringing notification with "Missed call".
func (s *Sender) Missed(line, callID string) {
	go s.ToLine(line, Notification{Kind: "missed", Title: "Missed call", Body: "A website visitor called. They may leave a message.",
		URL: "/app", Tag: "call-" + callID, Call: callID}, 24*time.Hour, false)
}

// Answered removes the ringing notification from the owner's other devices
// (the phone app that answered clears its own).
func (s *Sender) Answered(line, callID string) {
	go s.ToLine(line, Notification{Kind: "answered", Title: "Call answered", Body: "You answered this call on another device.",
		URL: "/app", Tag: "call-" + callID, Call: callID}, time.Minute, false)
}

// Voicemail says a new message is waiting.
func (s *Sender) Voicemail(line string) {
	go s.ToLine(line, Notification{Kind: "voicemail", Title: "New voicemail", Body: "A caller left you a message.",
		URL: "/app", Tag: "voicemail", Renotify: true}, 7*24*time.Hour, false)
}

var errGone = &statusError{http.StatusGone, "subscription gone"}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return http.StatusText(e.code) + ": " + e.msg }
