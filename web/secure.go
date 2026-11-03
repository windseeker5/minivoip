package web

import (
	"net/http"
	"strings"
)

// MaxBody is the most any request may send: the biggest upload is a logo.
// Handlers set smaller limits of their own.
const MaxBody = 16 << 20

// maxFormBody is the most a plain form may send (sign up, log in).
const maxFormBody = 64 << 10

// Secured wraps the whole site: the browser rules every page gets, and the
// limit on what a request can send.
func (s *Server) Secured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if secure(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		// Only the call window sits in a frame, on our own landing page (w.js);
		// on other sites it opens in its own window.
		framed := strings.HasPrefix(r.URL.Path, "/call/")
		if framed {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		} else {
			h.Set("X-Frame-Options", "DENY")
		}
		h.Set("Content-Security-Policy", s.policy(r, framed))
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
		}
		next.ServeHTTP(w, r)
	})
}

// policy says where a page may load things from: scripts only from this site,
// never inline, plus inline styles.
func (s *Server) policy(r *http.Request, framed bool) string {
	ancestors := "'none'"
	if framed {
		ancestors = "'self'"
	}
	connect := "'self' blob: wss://" + r.Host // blob: the logo cropper reads its own picture
	if s.Dev {
		connect += " ws://" + r.Host + " ws://localhost:* ws://127.0.0.1:*"
	}
	return "default-src 'self'" +
		"; script-src 'self'" +
		"; style-src 'self' 'unsafe-inline'" +
		"; font-src 'self'" +
		"; img-src 'self' data: blob:" +
		"; media-src 'self' blob:" +
		"; connect-src " + connect +
		"; worker-src 'self'" +
		"; frame-src 'self'" +
		"; frame-ancestors " + ancestors +
		"; object-src 'none'; base-uri 'self'; form-action 'self'"
}

// sandboxed marks a file callers uploaded: even if a browser took it for a web
// page, it couldn't run anything or reach the owner's login.
func sandboxed(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; media-src 'self'; sandbox")
}
