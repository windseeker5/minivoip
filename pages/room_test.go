package pages

import (
	"strings"
	"testing"
)

func TestRoomRenders(t *testing.T) {
	guest := render(t, Room(sampleCallLine(), false, "https://voip.example.com/room/auberge"))
	for _, want := range []string{
		"<title>Auberge des Pins · Meeting room</title>", `data-name`, ">Join<", "Waiting for Marie to open the room",
		`"isOwner":false`, `"url":"https://voip.example.com/room/auberge"`, "meet.js", "room.js",
	} {
		if !strings.Contains(guest, want) {
			t.Errorf("guest's room page is missing %q", want)
		}
	}
	if strings.Contains(guest, "You're the host") {
		t.Error("a guest must not be told they're the host")
	}
	host := render(t, Room(sampleCallLine(), true, "https://voip.example.com/room/auberge"))
	for _, want := range []string{"You're the host", "Open the room", `"isOwner":true`} {
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
