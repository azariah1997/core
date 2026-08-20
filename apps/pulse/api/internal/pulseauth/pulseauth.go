// Package pulseauth resolves the authenticated Core Platform caller for
// every Pulse API request. Pulse never implements its own authentication
// or validates Keycloak JWTs directly - per apps/pulse/docs/
// ARCHITECTURE_AUDIT.md's authorization model, it forwards the caller's
// own bearer token to Core's real GET /v1/users/me and trusts Core's
// answer, the same way a browser-based SDK consumer would. This also
// gets Pulse the same auto-provision-on-first-login behavior Core's own
// apps already rely on, for free.
package pulseauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
)

type callerCtxKey struct{}
type clientCtxKey struct{}
type tokenCtxKey struct{}

func WithCaller(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, callerCtxKey{}, userID)
}

func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(callerCtxKey{}).(string)
	return id, ok
}

// WithClient attaches an authenticated *coresdk.Client to ctx, so any
// module needing to act against Core on that same caller's behalf (e.g.
// pulse-connections calling Core's relationships API) reuses it instead
// of re-parsing the bearer token and building a second client.
// RequireUser calls this itself in production; exported so handler
// tests can inject a client pointed at a real local httptest.Server
// standing in for core-api, the same real-local-server testing
// convention this whole platform's SDKs use.
func WithClient(ctx context.Context, client *coresdk.Client) context.Context {
	return context.WithValue(ctx, clientCtxKey{}, client)
}

// ClientFromContext returns the per-request Core client authenticated
// as the current caller - never a fixed service-level credential.
func ClientFromContext(ctx context.Context) (*coresdk.Client, bool) {
	c, ok := ctx.Value(clientCtxKey{}).(*coresdk.Client)
	return c, ok
}

// WithToken attaches the caller's raw bearer token to ctx - needed
// (unlike WithClient's already-built core-api client) whenever a module
// must authenticate to a *different* Core service with the same
// identity, e.g. pulse-interactions checking realtime-gateway's own
// GET /v1/presence/{userId} as the current caller.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenCtxKey{}, token)
}

// TokenFromContext returns the caller's raw bearer token.
func TokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(tokenCtxKey{}).(string)
	return t, ok
}

// RequireUser returns middleware that resolves the caller's real Core
// User ID from the request's own Authorization header and attaches it
// (plus the authenticated client that resolved it) to the request
// context. coreAPIURL is Pulse's only network dependency for
// authentication - never Keycloak directly.
func RequireUser(coreAPIURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "missing bearer token"))
				return
			}
			client := coresdk.NewClient(coreAPIURL, coresdk.WithTokenSource(coresdk.StaticTokenSource(token)), coresdk.WithRetries(1, 0))
			user, err := client.UsersMe(r.Context())
			if err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "could not resolve caller against core-api"))
				return
			}
			ctx := WithCaller(r.Context(), user.ID)
			ctx = WithClient(ctx, client)
			ctx = WithToken(ctx, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	return token, token != ""
}
