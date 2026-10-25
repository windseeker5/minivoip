package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"minivoip/assets"
	"minivoip/calls"
	"minivoip/push"
	"minivoip/relay"
	"minivoip/store"
	"minivoip/web"
)

func main() {
	mux := http.NewServeMux()
	setupAssetsRoutes(mux)
	db, err := store.Open(dbPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer db.Close()

	// Calls: the socket that sets up calls between a visitor and the owner's
	// phone app (see calls/hub.go).
	hub := calls.NewHub(db.LineExists)
	if isDevelopment() {
		// The dev proxy (localhost:7331) sits in front of the app.
		hub.OriginPatterns = []string{"localhost:*", "127.0.0.1:*"}
	}
	// Push notifications wake the owner's phone (see push/push.go).
	sender, err := push.New(db, pushContact())
	if err != nil {
		fmt.Fprintln(os.Stderr, "push:", err)
		os.Exit(1)
	}
	hub.OnRing = sender.Ring
	hub.OnMissed = sender.Missed
	hub.OnAnswered = sender.Answered

	// The relay carries the voice when phones can't connect directly.
	// Meeting rooms (see calls/room.go); a guest waiting wakes the owner.
	rooms := calls.NewRooms(db.LineExists)
	rooms.OriginPatterns = hub.OriginPatterns
	rooms.OnKnock = sender.RoomKnock

	site := &web.Server{Store: db, Hub: hub, Rooms: rooms, Push: sender, Dev: isDevelopment(), OnVoicemail: sender.Voicemail, Admins: admins(), CallLine: callLine()}
	site.Relay = startRelay(db)
	site.Routes(mux)

	ln, err := listen()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Server is running on http://localhost:%d\n", ln.Addr().(*net.TCPAddr).Port)
	if err := http.Serve(ln, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// listen binds the port. An explicit PORT binds exactly (the templ proxy in
// the Taskfile points at it, so silently moving would break hot reload);
// without PORT it walks from 8090 to the next free port like node dev
// servers do.
func listen() (net.Listener, error) {
	if env := os.Getenv("PORT"); env != "" {
		port, err := strconv.Atoi(env)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT %q", env)
		}
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			return nil, fmt.Errorf("port %d is busy - stop the other app or run with another port, e.g. 'task dev PORT=%d'", port, port+1)
		}
		return ln, nil
	}
	for port := 8090; port < 8100; port++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port found from 8090 upwards")
}

func setupAssetsRoutes(mux *http.ServeMux) {
	development := isDevelopment()
	files := http.FileServer(http.FS(assets.Assets))
	if development {
		files = http.FileServer(http.Dir("./assets"))
	}
	assetHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if development {
			w.Header().Set("Cache-Control", "no-store")
		} else if strings.HasPrefix(strings.TrimPrefix(r.URL.Path, "/"), "js/shadcn-templ-") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", assetHandler))

	// Two files the phone app needs at the site's root: the service worker
	// (its scope is the folder it's served from) and the install manifest.
	at := func(file, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/" + file
			assetHandler.ServeHTTP(w, r2)
		}
	}
	mux.HandleFunc("GET /sw.js", at("js/sw.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("GET /manifest.webmanifest", at("brand/manifest.webmanifest", "application/manifest+json"))
}

// admins can manage promo codes: MINIVOIP_ADMINS (comma-separated emails).
// Users marked is_admin in the database are admins too.
func admins() []string {
	if a := os.Getenv("MINIVOIP_ADMINS"); a != "" {
		return strings.Split(a, ",")
	}
	return nil
}

// startRelay starts the built-in TURN relay on 3478 (voice on 49160-49200),
// reachable at MINIVOIP_PUBLIC_IP, else the address MINIVOIP_TURN_HOST
// (default voip.dresdell.com) points to. Without it, calls fall back to
// direct connections only.
func startRelay(db *store.Store) web.RelayInfo {
	secret, err := db.Setting("turn_secret")
	if err == nil && secret == "" {
		b := make([]byte, 24)
		rand.Read(b)
		secret = hex.EncodeToString(b)
		err = db.SetSetting("turn_secret", secret)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay secret:", err)
		return web.RelayInfo{}
	}
	host := os.Getenv("MINIVOIP_TURN_HOST")
	if host == "" {
		host = "voip.dresdell.com"
	}
	ip := net.ParseIP(os.Getenv("MINIVOIP_PUBLIC_IP"))
	if ip == nil {
		if ips, err := net.LookupIP(host); err == nil {
			for _, x := range ips {
				if x.To4() != nil {
					ip = x
					break
				}
			}
		}
	}
	if ip == nil {
		fmt.Fprintln(os.Stderr, "relay: no public address for", host, "- calls use direct connections only")
		return web.RelayInfo{}
	}
	if _, err := relay.Start(relay.Config{PublicIP: ip, Port: 3478, MinPort: 49160, MaxPort: 49200, Realm: host, Secret: secret}); err != nil {
		fmt.Fprintln(os.Stderr, "relay:", err, "- calls use direct connections only")
		return web.RelayInfo{}
	}
	return web.RelayInfo{Secret: secret, Host: host, Port: 3478}
}

// callLine is the line the landing page's "Call us now" button calls:
// MINIVOIP_CALL_LINE, else Ken's line.
func callLine() string {
	if l := os.Getenv("MINIVOIP_CALL_LINE"); l != "" {
		return l
	}
	return "ken"
}

// pushContact is the contact the push services see: MINIVOIP_PUSH_CONTACT,
// else the address on the landing page.
func pushContact() string {
	if c := os.Getenv("MINIVOIP_PUSH_CONTACT"); c != "" {
		return c
	}
	return "mailto:kdresdell@minipass.me"
}

// dbPath is the SQLite file: MINIVOIP_DB, else data/minivoip.db.
func dbPath() string {
	if p := os.Getenv("MINIVOIP_DB"); p != "" {
		return p
	}
	return "data/minivoip.db"
}

func isDevelopment() bool {
	// GO_ENV=development is a deprecated alias, removed after this minor version.
	return os.Getenv("SHADCN_TEMPL_DEV") == "true" || os.Getenv("TEMPL_DEV_MODE") == "true" || os.Getenv("GO_ENV") == "development"
}
