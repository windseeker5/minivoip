package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
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
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&sub)
	// The Android app sends its Firebase token as "fcm:<token>", no keys.
	android := strings.HasPrefix(sub.Endpoint, push.FCMPrefix) && len(sub.Endpoint) > len(push.FCMPrefix)
	if err != nil || !android && (!(strings.HasPrefix(sub.Endpoint, "https://") || s.Dev && strings.HasPrefix(sub.Endpoint, "http://localhost")) ||
		sub.Keys.P256dh == "" || sub.Keys.Auth == "") {
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

// pushReceipt is the phone's service worker saying it got a notification
// and what it did with it (store/pushlog.go). 404 means no such notification
// went to this account.
func (s *Server) pushReceipt(w http.ResponseWriter, r *http.Request, u store.User) {
	var rc struct {
		ID      int64  `json:"id"`
		ShownAs string `json:"shownAs"`
		Note    string `json:"note"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&rc); err != nil || rc.ID == 0 {
		http.Error(w, "Not a receipt.", http.StatusBadRequest)
		return
	}
	ok, err := s.Store.PushShown(u.AccountID, rc.ID, clip(rc.ShownAs, 20), clip(rc.Note, 200))
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// androidConfig hands the Android app its Firebase settings (push/fcm.go).
// 404 until the Firebase files are in place.
func (s *Server) androidConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := push.LoadAndroidConfig(push.FirebaseDir())
	if err != nil {
		http.Error(w, "Android ringing isn't set up on this server.", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

// androidAPK serves the Android app (android/, built into data/android).
func (s *Server) androidAPK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="minivoip.apk"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, filepath.Join("data", "android", "minivoip.apk"))
}

// declineCall is the Android app's Decline button on its ring screen.
func (s *Server) declineCall(w http.ResponseWriter, r *http.Request, u store.User) {
	l, err := s.Store.LineForAccount(u.AccountID)
	if err != nil {
		http.Error(w, "No line yet.", http.StatusBadRequest)
		return
	}
	s.Hub.Decline(l.Name, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
