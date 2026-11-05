package web

import (
	"fmt"
	"net/http"
	"strings"

	"rsc.io/qr"
)

// qrQuiet is the white margin a QR code needs around it, in squares.
const qrQuiet = 4

// androidQR is the QR code of the Android app's download, on this site's address
// as the visitor sees it. That address always serves the latest app, so a code
// never goes out of date (android/README.md).
func (s *Server) androidQR(w http.ResponseWriter, r *http.Request) {
	code, err := qr.Encode(baseURL(r)+"/download/minivoip.apk", qr.M)
	if err != nil {
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, qrSVG(code))
}

// qrSVG draws a code's black squares on white.
func qrSVG(code *qr.Code) string {
	var path strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+qrQuiet, y+qrQuiet)
			}
		}
	}
	size := code.Size + 2*qrQuiet
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#111113" d="%s"/></svg>`, size, size, size, size, path.String())
}
