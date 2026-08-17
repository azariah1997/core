package devices

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the device endpoints, all scoped to the caller's own
// user via requireUser (built in internal/api, which composes identity and
// users - this package only knows about users, not identity, matching the
// same decoupling users itself uses toward identity).
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/users/me/devices", requireUser(registerHandler(svc)))
	mux.Handle("GET /v1/users/me/devices", requireUser(listHandler(svc)))
	mux.Handle("DELETE /v1/users/me/devices/{id}", requireUser(revokeHandler(svc)))
}

type deviceResponse struct {
	ID             string        `json:"id"`
	ClientDeviceID string        `json:"clientDeviceId"`
	Platform       string        `json:"platform"`
	OSVersion      string        `json:"osVersion,omitempty"`
	AppVersion     string        `json:"appVersion,omitempty"`
	Locale         string        `json:"locale"`
	Timezone       string        `json:"timezone"`
	HasPushToken   bool          `json:"hasPushToken"`
	SessionStatus  SessionStatus `json:"sessionStatus"`
	LastActiveAt   string        `json:"lastActiveAt"`
	CreatedAt      string        `json:"createdAt"`
}

func toResponse(d Device) deviceResponse {
	return deviceResponse{
		ID: d.ID, ClientDeviceID: d.ClientDeviceID, Platform: d.Platform,
		OSVersion: d.OSVersion, AppVersion: d.AppVersion, Locale: d.Locale, Timezone: d.Timezone,
		HasPushToken: d.HasPushToken, SessionStatus: d.SessionStatus,
		LastActiveAt: d.LastActiveAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		CreatedAt:    d.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func registerHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := users.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
			return
		}
		var body struct {
			ClientDeviceID string `json:"clientDeviceId"`
			Platform       string `json:"platform"`
			OSVersion      string `json:"osVersion"`
			AppVersion     string `json:"appVersion"`
			Locale         string `json:"locale"`
			Timezone       string `json:"timezone"`
			PushToken      string `json:"pushToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		d, err := svc.Register(r.Context(), u.ID, RegisterInput{
			ClientDeviceID: body.ClientDeviceID, Platform: body.Platform,
			OSVersion: body.OSVersion, AppVersion: body.AppVersion,
			Locale: body.Locale, Timezone: body.Timezone, PushToken: body.PushToken,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(d))
	}
}

func listHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := users.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
			return
		}
		list, err := svc.List(r.Context(), u.ID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]deviceResponse, 0, len(list))
		for _, d := range list {
			items = append(items, toResponse(d))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func revokeHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := users.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
			return
		}
		if err := svc.Revoke(r.Context(), u.ID, r.PathValue("id")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "device not found"))
	default:
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
