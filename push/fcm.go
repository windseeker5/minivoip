package push

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The Android app (android/) gets its rings through Firebase Cloud Messaging:
// a high-priority FCM message wakes a sleeping phone, and the app rings like
// a phone call. Web push can only show a quiet notification.
//
// The Firebase project's two files live in one folder (MINIVOIP_FIREBASE_DIR,
// default data/firebase, never in git):
//   - google-services.json: the Android app's settings, handed to the app
//     (AndroidConfig) so the APK carries no Firebase settings of its own;
//   - service-account.json: the server's key for sending messages.
//
// A device whose endpoint starts with FCMPrefix is an Android app; the rest
// of the endpoint is its FCM registration token.

const (
	FCMPrefix  = "fcm:"
	AndroidApp = "com.kdresdell.minivoip"
)

// FirebaseDir is where the Firebase files are read from.
func FirebaseDir() string {
	if d := os.Getenv("MINIVOIP_FIREBASE_DIR"); d != "" {
		return d
	}
	return filepath.Join("data", "firebase")
}

// AndroidConfig is what the app needs to start Firebase.
type AndroidConfig struct {
	ProjectID string `json:"projectId"`
	AppID     string `json:"appId"`
	APIKey    string `json:"apiKey"`
	SenderID  string `json:"senderId"`
}

// LoadAndroidConfig reads google-services.json for the minivoip app.
func LoadAndroidConfig(dir string) (AndroidConfig, error) {
	b, err := os.ReadFile(filepath.Join(dir, "google-services.json"))
	if err != nil {
		return AndroidConfig{}, err
	}
	var g struct {
		ProjectInfo struct {
			ProjectNumber string `json:"project_number"`
			ProjectID     string `json:"project_id"`
		} `json:"project_info"`
		Client []struct {
			ClientInfo struct {
				AppID   string `json:"mobilesdk_app_id"`
				Android struct {
					Package string `json:"package_name"`
				} `json:"android_client_info"`
			} `json:"client_info"`
			APIKey []struct {
				Key string `json:"current_key"`
			} `json:"api_key"`
		} `json:"client"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		return AndroidConfig{}, err
	}
	for _, c := range g.Client {
		if c.ClientInfo.Android.Package != AndroidApp || len(c.APIKey) == 0 {
			continue
		}
		return AndroidConfig{ProjectID: g.ProjectInfo.ProjectID, AppID: c.ClientInfo.AppID,
			APIKey: c.APIKey[0].Key, SenderID: g.ProjectInfo.ProjectNumber}, nil
	}
	return AndroidConfig{}, fmt.Errorf("google-services.json has no %s app", AndroidApp)
}

// fcm sends FCM messages with the service account's key. The access token
// (one hour) is kept and renewed a minute early.
type fcm struct {
	dir    string
	client *http.Client
	api    string // "" is Google's; tests use their own

	mu      sync.Mutex
	acct    *serviceAccount
	token   string
	expires time.Time
}

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	key         *rsa.PrivateKey
}

var errNoFirebase = errors.New("firebase not set up (see push/fcm.go)")

// account loads service-account.json the first time it is there, so the
// files can be added while the server runs.
func (f *fcm) account() (*serviceAccount, error) {
	if f.acct != nil {
		return f.acct, nil
	}
	b, err := os.ReadFile(filepath.Join(f.dir, "service-account.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoFirebase
	} else if err != nil {
		return nil, err
	}
	var a serviceAccount
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(a.PrivateKey))
	if block == nil {
		return nil, errors.New("service-account.json: no private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	var ok bool
	if a.key, ok = k.(*rsa.PrivateKey); !ok {
		return nil, errors.New("service-account.json: not an RSA key")
	}
	if a.TokenURI == "" {
		a.TokenURI = "https://oauth2.googleapis.com/token"
	}
	f.acct = &a
	return f.acct, nil
}

// accessToken trades a signed JWT for an OAuth access token (Google's
// "service account" flow).
func (f *fcm) accessToken() (string, *serviceAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, err := f.account()
	if err != nil {
		return "", nil, err
	}
	if f.token != "" && time.Now().Before(f.expires) {
		return f.token, a, nil
	}
	now := time.Now()
	enc := base64.RawURLEncoding
	head := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss": a.ClientEmail, "scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud": a.TokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	unsigned := head + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", nil, err
	}
	res, err := f.client.PostForm(a.TokenURI, url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {unsigned + "." + enc.EncodeToString(sig)},
	})
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil || t.AccessToken == "" {
		return "", nil, fmt.Errorf("firebase token: %s", res.Status)
	}
	f.token, f.expires = t.AccessToken, now.Add(time.Duration(t.ExpiresIn)*time.Second-time.Minute)
	return f.token, a, nil
}

// send delivers one notification as an FCM data message (the app builds the
// notification itself, so a ring becomes a phone-call screen). It returns
// errGone when FCM says the app is no longer installed.
func (f *fcm) send(token string, n Notification, ttl time.Duration, urgent bool) error {
	access, a, err := f.accessToken()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(n)
	priority := "NORMAL"
	if urgent {
		priority = "HIGH"
	}
	msg, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token": token,
		"data":  map[string]string{"n": string(body)},
		"android": map[string]any{
			"priority":     priority,
			"ttl":          strconv.Itoa(int(ttl.Seconds())) + "s",
			"collapse_key": topic(n.Tag),
		},
	}})
	api := f.api
	if api == "" {
		api = "https://fcm.googleapis.com"
	}
	req, _ := http.NewRequest("POST", api+"/v1/projects/"+a.ProjectID+"/messages:send", bytes.NewReader(msg))
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 300 {
		return nil
	}
	reply, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
	if res.StatusCode == http.StatusNotFound || strings.Contains(string(reply), "UNREGISTERED") {
		return errGone
	}
	return &statusError{res.StatusCode, string(bytes.TrimSpace(reply[:min(len(reply), 300)]))}
}
