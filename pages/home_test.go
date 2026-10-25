package pages

import (
	"context"
	"io"
	"strings"
	"testing"
)

// The landing page must render without errors, with and without minivoip's
// own line to call.
func TestHomeRenders(t *testing.T) {
	if err := Home(false, "").Render(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Home(false, "ken").Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if strings.Count(page, `href="/call/ken"`) != 2 || !strings.Contains(page, "data-call-us") {
		t.Error(`"Call us now" should call minivoip's own line, in the hero and at the bottom`)
	}
	if strings.Contains(page, "Joe") {
		t.Error("the Joe's B&B sample is gone from the landing page")
	}
}
