package pages

import (
	"strings"
	"testing"
)

func TestRoomRenders(t *testing.T) {
	guest := render(t, Room(sampleCallLine(), false, "https://voip.example.com/room/auberge"))
	for _, want := range []string{
		"<title>Auberge des Pins · Meeting room</title>", `data-name`, ">Join<", "Waiting for Marie to let you in",
		`placeholder="So Marie knows who you are"`, `"isOwner":false`, `"url":"https://voip.example.com/room/auberge"`, "meet.js", "room.js",
	} {
		if !strings.Contains(guest, want) {
			t.Errorf("guest's room page is missing %q", want)
		}
	}
	if strings.Contains(guest, "data-door") {
		t.Error("a guest must not get the host's door")
	}
	if strings.Contains(guest, "You're the host") {
		t.Error("a guest must not be told they're the host")
	}
	host := render(t, Room(sampleCallLine(), true, "https://voip.example.com/room/auberge"))
	for _, want := range []string{"You're the host", "Open the room", `"isOwner":true`, "data-door"} {
		if !strings.Contains(host, want) {
			t.Errorf("owner's room page is missing %q", want)
		}
	}
	if strings.Contains(host, `data-name`) {
		t.Error("the owner isn't asked for a name")
	}
	// Like the call window: no site CSS, htmx or component scripts.
	for _, not := range []string{"output.css", "htmx", "shadcn"} {
		if strings.Contains(guest, not) {
			t.Errorf("room page should not load %s", not)
		}
	}
}

// Only the host's browser records: guests get no recorder.
func TestRoomRecorderIsTheHosts(t *testing.T) {
	l := sampleCallLine()
	l.Notes, l.Language = true, "fr"
	host := render(t, Room(l, true, "https://x/room/auberge"))
	if !strings.Contains(host, "recorder.js") || !strings.Contains(host, `"notes":true`) || !strings.Contains(host, "data-rec-note") {
		t.Error("the host's page doesn't record")
	}
	guest := render(t, Room(l, false, "https://x/room/auberge"))
	if strings.Contains(guest, "recorder.js") || strings.Contains(guest, `"notes":true`) || strings.Contains(guest, "data-rec-note") {
		t.Error("a guest's page records")
	}
	l.Notes = false
	if strings.Contains(render(t, Room(l, true, "https://x/room/auberge")), `"notes":true`) {
		t.Error("notes off: the host's page records")
	}
}
