package main

import (
	"net/http"

	"github.com/example/core-platform/backend/realtime-gateway/internal/identity"
	"github.com/example/core-platform/backend/realtime-gateway/internal/ws"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/jwtverify"
)

// identityProvider must match the provider name core-api's identity
// service registers under (see backend/core-api/cmd/server/main.go) -
// both resolve against the same identities table and must agree on what
// "the" provider is called.
const identityProvider = "keycloak"

// wsAuthMiddleware validates the token and resolves the platform user ID
// before the WebSocket upgrade happens. Browsers can't set an
// Authorization header on a WebSocket handshake, so the token travels as
// a query parameter instead - the standard fallback for this transport.
func wsAuthMiddleware(verifier *jwtverify.Verifier, resolver *identity.Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("access_token")
			deviceID := r.URL.Query().Get("deviceId")
			if token == "" || deviceID == "" {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "access_token and deviceId query parameters are required"))
				return
			}

			claims, err := verifier.Verify(r.Context(), token)
			if err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "invalid or expired token"))
				return
			}

			userID, err := resolver.UserIDForSubject(r.Context(), identityProvider, claims.Subject)
			if err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeUnauthenticated, "identity is not linked to a platform user yet"))
				return
			}

			next.ServeHTTP(w, r.WithContext(ws.WithAuth(r.Context(), ws.Auth{UserID: userID, DeviceID: deviceID})))
		})
	}
}
