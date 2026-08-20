package mood

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/coresdk"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// ConnectionsFactory builds a Connections bound to one caller's
// authenticated client - injected from the composition root
// (internal/api/router.go), which is the only place allowed to know the
// concrete adapter (over pulse-connections' real *Service), same
// import-cycle-avoidance reason every other module's *Factory type
// exists for.
type ConnectionsFactory func(client *coresdk.Client) Connections

// CirclesFactory builds a Circles bound to one caller's authenticated
// client - same reason as ConnectionsFactory.
type CirclesFactory func(client *coresdk.Client) Circles

func RegisterRoutes(mux *http.ServeMux, svc *Service, newConnections ConnectionsFactory, newCircles CirclesFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("PUT /v1/pulse/mood", requireUser(setHandler(svc, newConnections, newCircles)))
	mux.Handle("DELETE /v1/pulse/mood", requireUser(clearHandler(svc)))
	mux.Handle("GET /v1/pulse/mood/me", requireUser(myMoodHandler(svc)))
	mux.Handle("GET /v1/pulse/mood/{userId}", requireUser(getHandler(svc)))
}

type moodResponse struct {
	UserID string `json:"userId"`
	Emoji  string `json:"emoji"`
	// Audience/CustomUserIDs/UpdatedAt are the owner's own management
	// detail - never included when a viewer other than the owner is
	// looking at this Mood (see toMoodResponse).
	Audience      string   `json:"audience,omitempty"`
	CustomUserIDs []string `json:"customUserIds,omitempty"`
	CircleID      string   `json:"circleId,omitempty"`
	CreatedAt     string   `json:"createdAt"`
	ExpiresAt     string   `json:"expiresAt"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
}

func toMoodResponse(m Mood, ownerDetail bool) moodResponse {
	resp := moodResponse{
		UserID: m.UserID, Emoji: string(m.Emoji),
		CreatedAt: m.CreatedAt.UTC().Format(timeFormat), ExpiresAt: m.ExpiresAt.UTC().Format(timeFormat),
	}
	if ownerDetail {
		resp.Audience = string(m.Audience)
		resp.UpdatedAt = m.UpdatedAt.UTC().Format(timeFormat)
		if m.Audience == AudienceCustomUsers {
			resp.CustomUserIDs = m.AllowedViewerIDs
		}
		if m.Audience == AudienceSelectedCircles && m.CircleID != nil {
			resp.CircleID = *m.CircleID
		}
	}
	return resp
}

func setHandler(svc *Service, newConnections ConnectionsFactory, newCircles CirclesFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		client, ok := pulseauth.ClientFromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
			return
		}
		timezone := "UTC"
		if user, ok := pulseauth.UserFromContext(r.Context()); ok && user.Timezone != "" {
			timezone = user.Timezone
		}
		var body struct {
			Emoji         string   `json:"emoji"`
			Audience      string   `json:"audience"`
			CustomUserIDs []string `json:"customUserIds"`
			CircleID      string   `json:"circleId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		in := SetInput{Emoji: Emoji(body.Emoji), Audience: Audience(body.Audience), CustomUserIDs: body.CustomUserIDs, CircleID: body.CircleID}
		m, err := svc.Set(r.Context(), newConnections(client), newCircles(client), callerID, timezone, in)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toMoodResponse(m, true))
	}
}

func clearHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		if err := svc.Clear(r.Context(), callerID); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func myMoodHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		m, err := svc.MyMood(r.Context(), callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toMoodResponse(m, true))
	}
}

// getHandler serves another user's Mood - ErrNotFound (mapped to a
// plain 404) covers both "no Mood set" and "a Mood exists but the
// caller isn't in its audience", deliberately indistinguishable (see
// Service.Get's own doc comment).
func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		ownerID := r.PathValue("userId")
		m, err := svc.Get(r.Context(), callerID, ownerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toMoodResponse(m, callerID == ownerID))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "mood not found"))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled mood error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
