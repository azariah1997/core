// Package correlation propagates a correlation ID through context and HTTP
// headers so every request/event can be traced across services and logs.
package correlation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
)

type contextKey struct{}

// New generates a random correlation ID.
func New() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WithID stores a correlation ID in ctx.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the correlation ID stored in ctx, or "" if absent.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

// Middleware reads an inbound correlation/request ID header, generating one
// if neither is present, stores it in the request context, and reflects it
// back on both response headers.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderCorrelationID)
		if id == "" {
			id = r.Header.Get(HeaderRequestID)
		}
		if id == "" {
			id = New()
		}
		w.Header().Set(HeaderCorrelationID, id)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(WithID(r.Context(), id)))
	})
}
