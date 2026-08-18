package ws

import "context"

// Auth is who the connecting client authenticated as, resolved by
// whatever middleware wraps this package's Handler (validates the token,
// resolves the platform user ID, reads the client-supplied device ID).
type Auth struct {
	UserID   string
	DeviceID string
}

type authKey struct{}

func WithAuth(ctx context.Context, a Auth) context.Context {
	return context.WithValue(ctx, authKey{}, a)
}

func AuthFromContext(ctx context.Context) (Auth, bool) {
	a, ok := ctx.Value(authKey{}).(Auth)
	return a, ok
}
