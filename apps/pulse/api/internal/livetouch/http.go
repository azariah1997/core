package livetouch

import (
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

// PresenceFactory builds a Presence bound to the caller's raw bearer
// token - same shape as pulse-interactions' own PresenceFactory, since
// presence lives on realtime-gateway, a different base URL than core-api.
type PresenceFactory func(token string) Presence

// NotifierFactory builds a Notifier bound to one caller's authenticated
// core-api client.
type NotifierFactory func(client *coresdk.Client) Notifier

func RegisterRoutes(mux *http.ServeMux, svc *Service, newPresence PresenceFactory, newNotifier NotifierFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/live-touch/sessions", requireUser(inviteHandler(svc, newPresence, newNotifier)))
	mux.Handle("POST /v1/pulse/live-touch/sessions/{id}/accept", requireUser(acceptHandler(svc)))
	mux.Handle("POST /v1/pulse/live-touch/sessions/{id}/decline", requireUser(declineHandler(svc)))
	mux.Handle("POST /v1/pulse/live-touch/sessions/{id}/end", requireUser(endHandler(svc)))
	mux.Handle("GET /v1/pulse/live-touch/sessions/{id}", requireUser(getHandler(svc)))
}

type sessionResponse struct {
	ID           string  `json:"id"`
	OtherUserID  string  `json:"otherUserId"`
	Role         string  `json:"role"` // "initiator" or "receiver", from the caller's perspective
	Status       string  `json:"status"`
	EndReason    string  `json:"endReason,omitempty"`
	DeliveryMode string  `json:"deliveryMode"`
	Channel      string  `json:"channel,omitempty"` // only meaningful once active
	InvitedAt    string  `json:"invitedAt"`
	AcceptedAt   *string `json:"acceptedAt,omitempty"`
	EndedAt      *string `json:"endedAt,omitempty"`
	DurationMs   *int    `json:"durationMs,omitempty"`
}

func toSessionResponse(callerID string, s Session) sessionResponse {
	role := "initiator"
	if s.InitiatorID != callerID {
		role = "receiver"
	}
	resp := sessionResponse{
		ID: s.ID, OtherUserID: s.otherUser(callerID), Role: role,
		Status: string(s.Status), DeliveryMode: string(s.DeliveryMode), DurationMs: s.DurationMs,
		InvitedAt: s.InvitedAt.UTC().Format(timeFormat),
	}
	if s.EndReason != nil {
		resp.EndReason = string(*s.EndReason)
	}
	if s.Status == StatusActive {
		resp.Channel = s.Channel()
	}
	if s.AcceptedAt != nil {
		a := s.AcceptedAt.UTC().Format(timeFormat)
		resp.AcceptedAt = &a
	}
	if s.EndedAt != nil {
		e := s.EndedAt.UTC().Format(timeFormat)
		resp.EndedAt = &e
	}
	return resp
}

func inviteHandler(svc *Service, newPresence PresenceFactory, newNotifier NotifierFactory) http.HandlerFunc {
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
		token, ok := pulseauth.TokenFromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller token missing from context"))
			return
		}
		s, err := svc.Invite(r.Context(), newPresence(token), newNotifier(client), callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toSessionResponse(callerID, s))
	}
}

func acceptHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		s, err := svc.Accept(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSessionResponse(callerID, s))
	}
}

func declineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		s, err := svc.Decline(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSessionResponse(callerID, s))
	}
}

func endHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		s, err := svc.End(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSessionResponse(callerID, s))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		s, err := svc.Get(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSessionResponse(callerID, s))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "live touch session not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNotBonded):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrInvalidTransition):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrRateLimited):
		apperr.Write(w, r, apperr.New(apperr.CodeRateLimited, err.Error()))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled live touch error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
