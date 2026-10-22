// Package push wakes the owner's phone with web push notifications (the
// free VAPID standard: no Firebase or Apple account). The keys are made on
// first start and kept in the database's setting table.
package push

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"minivoip/store"
)

// Notification is the JSON the service worker (assets/js/sw.js) shows.
type Notification struct {
	Kind     string `json:"kind"` // ring, missed, voicemail, test
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	Tag      string `json:"tag"`      // same tag replaces the previous notification
	Renotify bool   `json:"renotify"` // sound and vibrate again when replacing
}

// RingEvery and RingFor shape the "ringing": a fresh notification every few
// seconds until the call is answered or stops ringing.
var (
	RingEvery = 5 * time.Second
	RingFor   = 30 * time.Second
)

type Sender struct {
	Store   *store.Store
	Subject string // "mailto:..." or https URL, sent to the push services
	// Client is used for tests; nil means the default HTTP client.
	Client webpush.HTTPClient

	public, private string
}

// New loads the VAPID keys, making them the first time.
func New(st *store.Store, subject string) (*Sender, error) {
	s := &Sender{Store: st, Subject: subject}
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

// ToLine sends a notification to every device of the line's owner and
// returns how many accepted it.
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
		go func(d store.Device) {
			defer wg.Done()
			if err := s.Send(d, n, ttl, urgent); err != nil {
				log.Printf("push: %s device %d: %v", line, d.ID, err)
				return
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}(d)
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
	n := Notification{Kind: "ring", Title: "Incoming call", Body: body, URL: "/app", Tag: "call-" + callID, Renotify: true}
	go func() {
		deadline := time.After(RingFor)
		tick := time.NewTicker(RingEvery)
		defer tick.Stop()
		for {
			s.ToLine(line, n, RingEvery+5*time.Second, true)
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
		URL: "/app", Tag: "call-" + callID}, 24*time.Hour, false)
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
