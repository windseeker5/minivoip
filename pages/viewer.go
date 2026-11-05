package pages

import "context"

type viewerKey struct{}

// Viewer is the logged-in user, as the page templates need it.
type Viewer struct {
	// Admin is minivoip staff: the account menu links to the admin page.
	Admin bool
	// NewMessages counts the unheard voicemails for the Messages tab; it is
	// only called when a dashboard page renders.
	NewMessages func() int
	// Premium says whether the account is on Premium, for the Notes tab's tag;
	// also only called when a dashboard page renders.
	Premium func() bool
}

// WithViewer tells the page templates who is logged in.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

func viewer(ctx context.Context) Viewer {
	v, _ := ctx.Value(viewerKey{}).(Viewer)
	return v
}

func isAdmin(ctx context.Context) bool { return viewer(ctx).Admin }

func onBasic(ctx context.Context) bool {
	f := viewer(ctx).Premium
	return f != nil && !f()
}

func newMessages(ctx context.Context) int {
	if f := viewer(ctx).NewMessages; f != nil {
		return f()
	}
	return 0
}
