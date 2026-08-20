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

type ctxKey struct{}

func WithCaller(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok
}

// RequireUser returns middleware that resolves the caller's real Core
// User ID from the request's own Authorization header and attaches it
// to the request context. coreAPIURL is Pulse's only network dependency
// for authentication - never Keycloak directly.
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
			next.ServeHTTP(w, r.WithContext(WithCaller(r.Context(), user.ID)))
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
