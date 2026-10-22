package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"minivoip/assets"
	"minivoip/calls"
	"minivoip/lines"
	"minivoip/pages"
)

func main() {
	mux := http.NewServeMux()
	setupAssetsRoutes(mux)
	mux.Handle("GET /", templ.Handler(pages.Home()))

	// Calls: the visitor's call window, the owner's phone app, and the
	// socket that sets up calls between them (see calls/hub.go).
	hub := calls.NewHub()
	if isDevelopment() {
		// The dev proxy (localhost:7331) sits in front of the app.
		hub.OriginPatterns = []string{"localhost:*", "127.0.0.1:*"}
	}
	mux.Handle("GET /ws", hub)
	mux.HandleFunc("GET /call/{line}", func(w http.ResponseWriter, r *http.Request) {
		l, ok := lines.Get(r.PathValue("line"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		templ.Handler(pages.Call(l)).ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /app", func(w http.ResponseWriter, r *http.Request) {
		// One test line until accounts exist (Phase 2).
		l, _ := lines.Get("joesbnb")
		templ.Handler(pages.Phone(l)).ServeHTTP(w, r)
	})

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
}

func isDevelopment() bool {
	// GO_ENV=development is a deprecated alias, removed after this minor version.
	return os.Getenv("SHADCN_TEMPL_DEV") == "true" || os.Getenv("TEMPL_DEV_MODE") == "true" || os.Getenv("GO_ENV") == "development"
}
