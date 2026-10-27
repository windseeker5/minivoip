package pages

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"minivoip/lines"
	"minivoip/store"
)

// sampleApp is a filled-in app: Joe's B&B with a day of calls.
func sampleApp() AppData {
	at := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.DateTime) }
	vm := func(id int64, d time.Duration, ms int64, listened bool, option string) *store.Voicemail {
		return &store.Voicemail{ID: id, Option: option, DurationMS: ms, ContentType: "audio/webm", Listened: listened, CreatedAt: at(d)}
	}
	v1, v2, v3 := vm(3, 20*time.Minute, 42000, false, "Book a room"), vm(2, 26*time.Hour, 19000, true, ""), vm(1, 30*time.Hour, 8000, true, "Leave a message")
	return AppData{
		Line: lines.Line{Name: "joesbnb", Business: "Joe's B&B", Greeting: "Hi, you've reached Joe's B&B.", Premium: true, Options: []lines.Option{
			{Key: "1", Label: "Book a room", Action: lines.Ring},
			{Key: "2", Label: "Opening hours", Action: lines.Play},
			{Key: "3", Label: "Our website", Action: lines.Link, URL: "https://joesbnb.ca"},
			{Key: "4", Label: "Leave a message", Action: lines.Voicemail},
		}},
		Email: "joe@example.com",
		History: []store.HistoryEntry{
			{Outcome: "missed", Option: "Book a room", At: at(21 * time.Minute), Voicemail: v1},
			{Outcome: "declined", Option: "Opening hours", At: at(80 * time.Minute)},
			{Outcome: "answered", Option: "Book a room", At: at(3 * time.Hour), TalkSeconds: 258},
			{Outcome: "away", At: at(25 * time.Hour)},
			{Outcome: "missed", At: at(26 * time.Hour), Voicemail: v2},
			{Option: "Leave a message", At: at(30 * time.Hour), Voicemail: v3},
		},
		Voicemails:  []store.Voicemail{*v1, *v2, *v3},
		VapidKey:    "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U",
		CallURL:     "voip.example.com/call/joesbnb",
		RoomURL:     "https://voip.example.com/room/joesbnb",
		Devices:     2,
		RingSeconds: 30,
		Android:     "0.3",
	}
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestAppRenders(t *testing.T) {
	d := sampleApp()
	html := render(t, Phone(d))
	for _, want := range []string{`data-tab="calls"`, `data-panel="me"`, `data-call-screen`, `data-push-state`,
		"Call while you were away", "Declined", "Answered", "4:18", "Yesterday", "Opens https://joesbnb.ca", `data-new`,
		`data-panel="room"`, "https://voip.example.com/room/joesbnb", `data-room-alert`} {
		if !strings.Contains(html, want) {
			t.Errorf("app is missing %q", want)
		}
	}
	if strings.Contains(html, `href="/dashboard"`) {
		t.Error("the app must not link to the dashboard")
	}
	empty := render(t, AppCalls(nil))
	if !strings.Contains(empty, "No calls yet") {
		t.Error("empty call list")
	}
	if got := render(t, Availability(store.Away)); !strings.Contains(got, "Off") || !strings.Contains(got, `data-mode="off"`) {
		t.Errorf("Availability(Away) = %s", got)
	}
	// Someone waiting in the meeting room shows when the app opens.
	d.RoomWaiting = []string{"Ana", "Bo", "Cy"}
	if got := render(t, Phone(d)); !strings.Contains(got, "Ana and 2 others are waiting in your meeting room") {
		t.Error("the app doesn't say who waits in the meeting room")
	}
	// Basic: no video tools in calls; the Room tab says what Premium adds.
	d.Line.Premium = false
	basic := render(t, Phone(d))
	for _, not := range []string{`data-action="camera"`, `data-action="board"`, `data-room-open`, `data-room-url`, `data-room-alert`, "/room/"} {
		if strings.Contains(basic, not) {
			t.Errorf("a Basic app shows %q", not)
		}
	}
	if !strings.Contains(basic, `data-room-locked`) || !strings.Contains(basic, "You're on Basic") {
		t.Error("a Basic app's Room tab should say what Premium adds")
	}
}

// With MINIVOIP_PREVIEW=<dir>, writes the app and its fragments there for a
// browser check without the server (tests/e2e/app.mjs).
func TestAppPreview(t *testing.T) {
	dir := os.Getenv("MINIVOIP_PREVIEW")
	if dir == "" {
		t.Skip("set MINIVOIP_PREVIEW to write preview files")
	}
	d := sampleApp()
	empty := d
	empty.History, empty.Voicemails = nil, nil
	basic := d
	basic.Line.Premium = false
	files := map[string]templ.Component{
		"app.html":         Phone(d),
		"app-empty.html":   Phone(empty),
		"app-basic.html":   Phone(basic),
		"calls.html":       AppCalls(d.History),
		"voicemails.html":  AppVoicemails(d.Voicemails),
		"avail-on.html":    Availability(""),
		"avail-pause.html": Availability(time.Now().Add(time.Hour).UTC().Format(time.DateTime)),
		"avail-off.html":   Availability(store.Away),
		"login.html":       AppLogin(AccountForm{Next: "/app", Email: "joe@example.com", Error: "Wrong email or password."}),
	}
	for name, c := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(render(t, c)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
