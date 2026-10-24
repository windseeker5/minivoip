package push

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"minivoip/store"
)

// firebaseFiles writes a google-services.json and a service-account.json
// whose token address is tokenURL.
func firebaseFiles(t *testing.T, tokenURL string) string {
	t.Helper()
	dir := t.TempDir()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	sa, _ := json.Marshal(map[string]string{"project_id": "minivoip-test", "client_email": "push@minivoip-test.example",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": tokenURL})
	os.WriteFile(filepath.Join(dir, "service-account.json"), sa, 0o600)
	os.WriteFile(filepath.Join(dir, "google-services.json"), []byte(`{
	  "project_info": {"project_number": "123", "project_id": "minivoip-test"},
	  "client": [
	    {"client_info": {"mobilesdk_app_id": "1:123:android:other", "android_client_info": {"package_name": "com.example.other"}}, "api_key": [{"current_key": "nope"}]},
	    {"client_info": {"mobilesdk_app_id": "1:123:android:abc", "android_client_info": {"package_name": "com.kdresdell.minivoip"}}, "api_key": [{"current_key": "AIzaTest"}]}
	  ]}`), 0o600)
	return dir
}

func TestAndroidConfig(t *testing.T) {
	cfg, err := LoadAndroidConfig(firebaseFiles(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := AndroidConfig{ProjectID: "minivoip-test", AppID: "1:123:android:abc", APIKey: "AIzaTest", SenderID: "123"}
	if cfg != want {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
	if _, err := LoadAndroidConfig(t.TempDir()); err == nil {
		t.Fatal("no file should be an error")
	}
}

// A ring to an Android device goes out as a high-priority FCM message; an
// app that is gone (UNREGISTERED) is forgotten.
func TestFCMRing(t *testing.T) {
	tokens := 0
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			r.ParseForm()
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
				t.Errorf("bad token request %v", r.Form)
			}
			tokens++
			w.Write([]byte(`{"access_token":"at","expires_in":3600}`))
		case r.URL.Path == "/v1/projects/minivoip-test/messages:send":
			if r.Header.Get("Authorization") != "Bearer at" {
				t.Errorf("no access token: %v", r.Header)
			}
			var m struct {
				Message struct {
					Token   string            `json:"token"`
					Data    map[string]string `json:"data"`
					Android struct {
						Priority string `json:"priority"`
					} `json:"android"`
				} `json:"message"`
			}
			json.NewDecoder(r.Body).Decode(&m)
			if m.Message.Token == "gone" {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`))
				return
			}
			var n Notification
			json.Unmarshal([]byte(m.Message.Data["n"]), &n)
			if m.Message.Token != "tok1" || m.Message.Android.Priority != "HIGH" || n.Kind != "ring" || n.Call != "c1" {
				t.Errorf("bad message %+v / %+v", m, n)
			}
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer google.Close()

	st, s := setup(t, "https://push.example/unused")
	s.Store.DeleteDevice("https://push.example/unused")
	u, _ := st.Login("joe@example.com", "password1")
	st.SaveDevice(u.AccountID, store.Device{Endpoint: FCMPrefix + "tok1"})
	st.SaveDevice(u.AccountID, store.Device{Endpoint: FCMPrefix + "gone"})
	s.fcm = &fcm{dir: firebaseFiles(t, google.URL+"/token"), client: google.Client(), api: google.URL}

	n := s.ToLine("joesbnb", Notification{Kind: "ring", Call: "c1", Tag: "call-c1"}, time.Minute, true)
	if n != 1 {
		t.Fatalf("delivered to %d devices, want 1", n)
	}
	if tokens != 1 {
		t.Errorf("asked for %d access tokens, want 1 (kept)", tokens)
	}
	if left := st.DeviceCount(u.AccountID); left != 1 {
		t.Errorf("%d devices left, want the gone app forgotten", left)
	}
}

// Without the Firebase files, Android devices just don't get pushes.
func TestFCMNotSetUp(t *testing.T) {
	_, s := setup(t, FCMPrefix+"tok1")
	s.fcm = &fcm{dir: t.TempDir(), client: http.DefaultClient}
	if n := s.ToLine("joesbnb", Notification{Kind: "test"}, time.Minute, true); n != 0 {
		t.Fatalf("delivered %d", n)
	}
}

// With MINIVOIP_FCM_LIVE=<firebase dir>, checks the real Firebase key: a
// made-up token must be refused as a bad token, not for a bad key.
func TestFCMLive(t *testing.T) {
	dir := os.Getenv("MINIVOIP_FCM_LIVE")
	if dir == "" {
		t.Skip("set MINIVOIP_FCM_LIVE to the Firebase folder")
	}
	f := &fcm{dir: dir, client: http.DefaultClient}
	err := f.send("not-a-real-token", Notification{Kind: "test"}, time.Minute, false)
	t.Logf("FCM said: %v", err)
	if err == nil || strings.Contains(err.Error(), "Unauthorized") || strings.Contains(err.Error(), "Forbidden") ||
		strings.Contains(err.Error(), "firebase token") {
		t.Fatal("the Firebase key doesn't work")
	}
}
