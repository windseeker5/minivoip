package pages

import (
	"context"
	"io"
	"testing"
)

// The landing page must render without errors.
func TestHomeRenders(t *testing.T) {
	if err := Home().Render(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
}
