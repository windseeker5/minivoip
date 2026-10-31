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
	if !strings.Contains(page, `src="/prompt/prompt-x.webm"`) || !strings.Contains(page, "data-voice-play") {
		t.Error("the hero should play the line's recorded greeting")
	}
	if !strings.Contains(page, "Book a demo") || !strings.Contains(page, "you&#39;ve") && !strings.Contains(page, "you've") {
		t.Error("the hero should write out the line's greeting and menu")
	}
	if !strings.Contains(page, "https://voip.example/call/") {
		t.Error("the code to paste should use this site's address")
	}
	if strings.Contains(page, "Joe") {
		t.Error("the Joe's B&B sample is gone from the landing page")
	}
}
