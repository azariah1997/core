package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

type contextKey struct{}

// FromContext returns the Identity attached by Middleware, if any.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(contextKey{}).(Identity)
	return id, ok
}

// Middleware requires a valid Bearer token on every request it wraps,
// resolving it to a platform Identity via svc and attaching it to the
// request context. Routes that don't need authentication should not be
// wrapped by it - it has no notion of "optional" auth.
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "missing or malformed Authorization header"))
				return
			}
			id, err := svc.Authenticate(r.Context(), token)
			if err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "invalid or expired token"))
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

// RegisterRoutes wires a minimal identity-inspection endpoint. It exists so
// the whole authenticate -> resolve -> attach pipeline is independently
// verifiable before any product endpoint requires authentication.
func RegisterRoutes(mux *http.ServeMux, svc *Service) {
	mux.Handle("GET /v1/identity/me", Middleware(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "identity missing from context"))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"id":              id.ID,
			"provider":        id.Provider,
			"providerSubject": id.ProviderSubject,
			"status":          id.Status,
			"userId":          id.UserID,
		})
	})))
}
