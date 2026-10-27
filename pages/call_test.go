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
	return lines.Line{Name: "auberge", Business: "Auberge des Pins", Owner: "Marie", Greeting: "Hi, you've reached Auberge des Pins.", Premium: true,
		Options: []lines.Option{
			{Key: "1", Label: "Book a room", Action: lines.Ring, Say: "Please hold, I'm getting Marie."},
			{Key: "2", Label: "Hours and directions", Action: lines.Play, Say: "We're at 12 chemin du Lac in Magog. Check-in from 3 pm."},
			{Key: "3", Label: "See our rooms", Action: lines.Link, URL: "https://example.com/rooms", Say: "Opening our rooms page."},
			{Key: "4", Label: "Leave a message", Action: lines.Voicemail},
		}}
}

func TestCallRenders(t *testing.T) {
	page := render(t, Call(sampleCallLine(), 30, true))
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
	for _, want := range []string{`data-action="camera"`, `data-action="board"`, "is-meet"} {
		if !strings.Contains(page, want) {
			t.Errorf("a Premium line's call is missing %q", want)
		}
	}
	// Basic: voice only.
	basic := sampleCallLine()
	basic.Premium = false
	page = render(t, Call(basic, 30, true))
	for _, not := range []string{`data-action="camera"`, `data-action="screen"`, `data-action="board"`, "is-meet"} {
		if strings.Contains(page, not) {
			t.Errorf("a Basic line's call shows %q", not)
		}
	}
	if !strings.Contains(page, `data-action="mute"`) || !strings.Contains(page, `data-action="hangup"`) {
		t.Error("a Basic call still has Mute and Hang up")
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
		"call.html":      render(t, Call(sampleCallLine(), 30, true)),
		"call-one.html":  render(t, Call(one, 30, true)),
		"call-many.html": render(t, Call(many, 30, true)),
	}
	for name, page := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A business with a logo shows it instead of its initials: on the call
// window, in the meeting room, and as the owner's face on the stage.
func TestLogoShows(t *testing.T) {
	l := sampleCallLine()
	l.Logo = "logo-0123456789abcdef0123456789abcdef.png"
	src := `src="/logo/logo-0123456789abcdef0123456789abcdef.png"`
	page := render(t, Call(l, 30, true))
	if strings.Count(page, src) != 5 || strings.Contains(page, ">AP<") || strings.Contains(page, ">M<") {
		t.Errorf("call page: %d logos, initials still there: %v", strings.Count(page, src), strings.Contains(page, ">AP<"))
	}
	if !strings.Contains(page, `"logo":"/logo/logo-0123456789abcdef0123456789abcdef.png"`) {
		t.Error("call.js isn't given the logo for the owner's face")
	}
	host := render(t, Room(l, true, "https://voip.example.com/room/auberge"))
	if strings.Count(host, src) != 3 || !strings.Contains(host, `"logo":"/logo/`) {
		t.Errorf("room page: %d logos", strings.Count(host, src))
	}
	dash := render(t, Dashboard("marie@example.com", l, "https://x/call/auberge", "https://x/room/auberge", 0, "Plan: Premium", "saved"))
	for _, want := range []string{src, "Change logo", "Remove the logo", "Callers see your new logo right away", `enctype="multipart/form-data"`} {
		if !strings.Contains(dash, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
	l.Logo = ""
	dash = render(t, Dashboard("marie@example.com", l, "https://x/call/auberge", "https://x/room/auberge", 0, "Plan: Premium", "bad"))
	for _, want := range []string{"Add my logo", "That file", ">AP<"} {
		if !strings.Contains(dash, want) {
			t.Errorf("dashboard without a logo is missing %q", want)
		}
	}
	if strings.Contains(dash, "Remove the logo") {
		t.Error("no logo to remove")
	}
	if !strings.Contains(render(t, NewLine("marie@example.com", LineForm{Host: "x"})), `name="logo"`) {
		t.Error("creating a line doesn't ask for the logo")
	}
}

// Away (Available switch off, or no phone online): the call page says so and
// offers the menu; a ringing choice says it leads to a message.
func TestCallAway(t *testing.T) {
	page := render(t, Call(sampleCallLine(), 30, false))
	for _, want := range []string{`data-here="false"`, "Away right now", "Hear our menu", "Away now: leave a message", "Marie is here"} {
		if !strings.Contains(page, want) {
			t.Errorf("away call page is missing %q", want)
		}
	}
	if !strings.Contains(render(t, Call(sampleCallLine(), 30, true)), `data-here="true"`) {
		t.Error("a call page with the owner here says so")
	}
	dash := render(t, Dashboard("marie@example.com", sampleCallLine(), "https://voip.example.com/call/auberge", "https://voip.example.com/room/auberge", 0, "", ""))
	if !strings.Contains(dash, `value="&lt;script src=&#34;https://voip.example.com/w.js&#34; data-line=&#34;auberge&#34; async&gt;&lt;/script&gt;"`) {
		t.Error("the dashboard doesn't give the Call button's line to paste")
	}
}
