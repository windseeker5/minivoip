package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"minivoip/push"
	"minivoip/store"
)

// pushSubscribe saves the phone app's PushSubscription for this account.
func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request, u store.User) {
	var sub struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&sub); err != nil ||
		!(strings.HasPrefix(sub.Endpoint, "https://") || s.Dev && strings.HasPrefix(sub.Endpoint, "http://localhost")) ||
		sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		http.Error(w, "Not a push subscription.", http.StatusBadRequest)
		return
	}
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	if err := s.Store.SaveDevice(u.AccountID, store.Device{Endpoint: sub.Endpoint, P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth, UserAgent: ua}); err != nil {
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request, u store.User) {
	var sub struct {
		Endpoint string `json:"endpoint"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&sub)
	s.Store.DeleteDevice(sub.Endpoint)
	w.WriteHeader(http.StatusNoContent)
}

// pushTest sends "This is a test" to all of the account's devices.
func (s *Server) pushTest(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if err != nil || s.Push == nil {
		http.Error(w, "No line yet.", http.StatusBadRequest)
		return
	}
	n := s.Push.ToLine(l.Name, push.Notification{Kind: "test", Title: "minivoip test", Body: "Notifications work. Calls will wake this phone.",
		URL: "/app", Tag: "test", Renotify: true}, time.Minute, true)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"sent":%d,"devices":%d}`, n, s.Store.DeviceCount(u.AccountID))
}
