package pages

import (
	"context"
	"io"
	"strings"
	"testing"

	"minivoip/lines"
)

// The landing page must render without errors, with and without minivoip's
// own line to call.
// Notes are sold as built, in the dashboard, with their limit, and the page says
// plainly that calls can be recorded.
func TestHomeOnNotes(t *testing.T) {
	var b strings.Builder
	if err := Home(false, nil, "https://voip.example").Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	for _, want := range []string{"Notes after each call, meeting and voicemail", "In your dashboard, up to 10 hours a month", "Are calls recorded?", "Callers are told first"} {
		if !strings.Contains(page, want) {
			t.Errorf("the landing page doesn't say %q", want)
		}
	}
	for _, not := range []string{">Soon<", "in your app after each call", "when the call notes are ready", "othing is recorded"} {
		if strings.Contains(page, not) {
			t.Errorf("the landing page still says %q", not)
		}
	}
}

func TestHomeRenders(t *testing.T) {
	if err := Home(false, nil, "https://voip.example").Render(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	line := &lines.Line{Name: "ken", Greeting: "Hi, you've reached minivoip.", GreetingAudio: "prompt-x.webm",
		Options: []lines.Option{{Key: "1", Label: "Book a demo", Action: lines.Ring}}}
	var b strings.Builder
	if err := Home(false, line, "https://voip.example").Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if strings.Count(page, `href="/call/ken"`) != 3 || !strings.Contains(page, "data-call-us") {
		t.Error(`"Call us now" should call minivoip's own line: the hero, the questions' "ask me" and the bottom`)
	}
	if strings.Contains(strings.ToLower(page), "free while in beta") || strings.Contains(page, "free line") {
		t.Error("Basic isn't free any more; only a promo code makes it free")
	}
	if !strings.Contains(page, `action="/premium"`) || !strings.Contains(page, "$10") {
		t.Error("Premium is $10 and starts with an email (POST /premium)")
	}
	if !strings.Contains(page, "Let people call you from your website, in one click.") || strings.Count(page, `class="lp-step"`) != 5 {
		t.Error("the hero should say it in one line, then show the call in five steps")
	}
	if !strings.Contains(page, "Show them your screen") || !strings.Contains(page, "<em>Premium</em>") {
		t.Error("the hero's last step should be Premium's: show them your screen")
	}
	if strings.Contains(page, "data-voice") {
		t.Error("the hero's voice player is gone")
	}
	if !strings.Contains(page, "https://voip.example/call/") {
		t.Error("the code to paste should use this site's address")
	}
	if strings.Contains(page, "Joe") {
		t.Error("the Joe's B&B sample is gone from the landing page")
	}
}
