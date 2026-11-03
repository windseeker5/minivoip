// Package push wakes the owner's phone: web push notifications (the free
// VAPID standard) for the browser app, and Firebase (fcm.go) for the
// Android app, which can ring like a phone call. The VAPID keys are made on
// first start and kept in the database's setting table.
package push

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
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
	// Icon is the business's logo (/logo/<name>), which the service worker
	// shows round; "" shows the minivoip mark. ToLine fills it in.
	Icon string `json:"icon,omitempty"`
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
	// Client sends web push; New gives it a time limit and no redirects.
	Client webpush.HTTPClient
	// Allowed says whether a browser's push address may be sent to. New sets
	// AllowedEndpoint; nil allows every address (tests).
	Allowed func(endpoint string) bool

	public, private string
	fcm             *fcm
}

// New loads the VAPID keys, making them the first time.
func New(st *store.Store, subject string) (*Sender, error) {
	s := &Sender{Store: st, Subject: subject, Allowed: AllowedEndpoint, fcm: &fcm{dir: FirebaseDir(), client: &http.Client{Timeout: 15 * time.Second}}}
	s.Client = &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
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

// pushHosts are the browser makers' push services: Chrome and other Chromium
// browsers, Firefox, Safari, Edge. Exact name, or a name ending in ".<host>".
var pushHosts = []string{"fcm.googleapis.com", "android.googleapis.com", "push.services.mozilla.com", "push.apple.com", "notify.windows.com"}

// AllowedEndpoint says whether a browser's push address is really one of the
// push services. The address comes from the browser, and the server sends to
// it, so anything else could aim this server at the home network.
func AllowedEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range pushHosts {
		if host == h || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}

// LocalEndpoint says whether a push address is on this machine. Only the
// browser test's fake push service, in development, from this machine.
func LocalEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "http" && u.User == nil && u.Hostname() == "localhost"
}

var errNotPush = errors.New("not a push service")

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
	if s.Allowed != nil && !s.Allowed(d.Endpoint) {
		s.Store.DeleteDevice(d.Endpoint)
		return errNotPush
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
	if n.Icon == "" {
		_, n.Icon = s.about(line)
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

// about gives the line's business name and logo address, for notifications.
func (s *Sender) about(line string) (business, icon string) {
	l, err := s.Store.LineByName(line)
	if err != nil {
		return "", ""
	}
	if l.Logo != "" {
		icon = "/logo/" + l.Logo
	}
	return l.Business, icon
}

// titled names the business after what happened ("Incoming call: Joe's
// B&B"), so an owner with several lines knows which one.
func (s *Sender) titled(what, line string) (title, icon string) {
	business, icon := s.about(line)
	if business == "" {
		return what, icon
	}
	return what + ": " + business, icon
}

// Ring notifies the owner's devices now and every RingEvery until stop is
// called or RingFor has passed. It returns at once; stop is safe to call
// more than once. The caller's menu choice isn't shown: its label ("Talk to
// Joe") reads oddly to Joe.
func (s *Sender) Ring(line, callID, _ string) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	title, icon := s.titled("Incoming call", line)
	n := Notification{Kind: "ring", Title: title, Body: "Someone on your website wants to talk to you.", Icon: icon,
		URL: "/app", Tag: "call-" + callID, Renotify: true, Call: callID, Until: time.Now().Add(RingFor).UnixMilli()}
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
	go func() {
		title, icon := s.titled("Missed call", line)
		s.ToLine(line, Notification{Kind: "missed", Title: title, Body: "Someone called from your website. They may leave a message.",
			Icon: icon, URL: "/app", Tag: "call-" + callID, Call: callID}, 24*time.Hour, false)
	}()
}

// Answered removes the ringing notification from the owner's other devices
// (the phone app that answered clears its own).
func (s *Sender) Answered(line, callID string) {
	go s.ToLine(line, Notification{Kind: "answered", Title: "Call answered", Body: "You answered this call on another device.",
		URL: "/app", Tag: "call-" + callID, Call: callID}, time.Minute, false)
}

// Voicemail says a new message is waiting.
func (s *Sender) Voicemail(line string) {
	go func() {
		title, icon := s.titled("New voicemail", line)
		s.ToLine(line, Notification{Kind: "voicemail", Title: title, Body: "A caller left you a message.",
			Icon: icon, URL: "/app", Tag: "voicemail", Renotify: true}, 7*24*time.Hour, false)
	}()
}

// RoomKnock says a guest is waiting in the meeting room.
func (s *Sender) RoomKnock(line, name string) {
	go s.ToLine(line, Notification{Kind: "room", Title: "Someone is waiting in your meeting room", Body: name + " is waiting for you. Tap to open the room.",
		URL: "/room/" + line, Tag: "room", Renotify: true}, 10*time.Minute, true)
}

var errGone = &statusError{http.StatusGone, "subscription gone"}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return http.StatusText(e.code) + ": " + e.msg }
