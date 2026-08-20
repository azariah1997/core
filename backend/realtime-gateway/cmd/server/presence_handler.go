package main

import (
	"net/http"

	"github.com/example/core-platform/backend/realtime-gateway/internal/presence"
	"github.com/example/core-platform/backend/realtime-gateway/internal/ws"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// presenceHandler answers "is this user currently connected to
// realtime-gateway, on any device" - a coarse online/offline signal,
// never exact activity or a last-seen timestamp (product spec §28's own
// privacy line) - for a product backend's own delivery-routing
// decisions (Pulse's Phase 5 live-vs-push fallback is the first real
// caller). Requires authentication (any real platform user) but no
// relationship to the queried user - the same coarse trust level
// realtime delivery itself already has: any authenticated backend can
// already push to any user via rtbus with no relationship check, so
// asking "are they online" is not a bigger disclosure than that.
func presenceHandler(tracker *presence.Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ws.AuthFromContext(r.Context()); !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		userID := r.PathValue("userId")
		online, err := tracker.IsOnline(r.Context(), userID)
		if err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "failed to check presence"))
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"userId": userID, "online": online})
	}
}
