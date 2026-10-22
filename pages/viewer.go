package pages

import "context"

type viewerKey struct{}

// WithAdmin tells the page templates whether the logged-in user is an admin
// (the dashboard shows an Admin link).
func WithAdmin(ctx context.Context, admin bool) context.Context {
	return context.WithValue(ctx, viewerKey{}, admin)
}

func isAdmin(ctx context.Context) bool {
	admin, _ := ctx.Value(viewerKey{}).(bool)
	return admin
}
