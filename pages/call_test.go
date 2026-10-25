package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"minivoip/lines"
)

// sampleCallLine is an example business with one choice of each kind.
func sampleCallLine() lines.Line {
	return lines.Line{Name: "auberge", Business: "Auberge des Pins", Owner: "Marie", Greeting: "Hi, you've reached Auberge des Pins.",
		Options: []lines.Option{
			{Key: "1", Label: "Book a room", Action: lines.Ring, Say: "Please hold, I'm getting Marie."},
			{Key: "2", Label: "Hours and directions", Action: lines.Play, Say: "We're at 12 chemin du Lac in Magog. Check-in from 3 pm."},
			{Key: "3", Label: "See our rooms", Action: lines.Link, URL: "https://example.com/rooms", Say: "Opening our rooms page."},
			{Key: "4", Label: "Leave a message", Action: lines.Voicemail},
		}}
}

func TestCallRenders(t *testing.T) {
	page := render(t, Call(sampleCallLine(), 30))
	for _, want := range []string{
		"<title>Call Auberge des Pins</title>",
		`data-key="1"`, `data-key="4"`, "Live call", "Hear the answer", "Opens a page", "Up to 2 minutes",
		"Before you talk to Marie, your browser will ask to use your microphone.",
		"Say your name and phone number so Marie can call you back.",
		`"ringSeconds":30`, ">AP<",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("call page is missing %q", want)
		}
	}
	// The call page loads only what it needs: no site CSS, htmx or component scripts.
	for _, not := range []string{"output.css", "htmx", "shadcn"} {
		if strings.Contains(page, not) {
			t.Errorf("call page should not load %s", not)
		}
	}
	// Mute shows only during the call, never on the menu.
	menu := page[strings.Index(page, `data-show="menu"`):strings.Index(page, `data-show="ringing connecting"`)]
	if strings.Contains(menu, `data-action="mute"`) {
		t.Error("the menu must not show Mute")
	}
}

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{"Auberge des Pins": "AP", "OMT": "O", "Joe's B&B": "JB", "  ": "?", "the Lake Shop": "LS"} {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}

// With MINIVOIP_PREVIEW=<dir>, writes call windows there for a browser check
// without the server (tests/e2e/call.mjs).
func TestCallPreview(t *testing.T) {
	dir := os.Getenv("MINIVOIP_PREVIEW")
	if dir == "" {
		t.Skip("set MINIVOIP_PREVIEW to write preview files")
	}
	one := lines.Line{Name: "omt", Business: "OMT", Owner: "OMT", Greeting: "Hi, you've reached OMT.",
		Options: []lines.Option{{Key: "0", Label: "Talk to Ken", Action: lines.Ring}}}
	many := sampleCallLine()
	many.Business = "Les Gîtes et Chalets du Grand Lac Memphrémagog"
	for _, k := range []string{"5", "6", "7"} {
		many.Options = append(many.Options, lines.Option{Key: k, Label: "Group bookings and special events", Action: lines.Play, Say: "Call us."})
	}
	files := map[string]string{
		"call.html":      render(t, Call(sampleCallLine(), 30)),
		"call-one.html":  render(t, Call(one, 30)),
		"call-many.html": render(t, Call(many, 30)),
	}
	for name, page := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
