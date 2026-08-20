package pulseprofile

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// RegisterRoutes wires the pulse-profile endpoints, all requiring an
// authenticated Core caller (pulseauth.RequireUser resolves them
// against Core's own GET /v1/users/me - see that package's doc comment
// for why Pulse never validates Keycloak JWTs itself).
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/profile", requireUser(ensureProfileHandler(svc)))
	mux.Handle("GET /v1/pulse/profile/me", requireUser(getMyProfileHandler(svc)))
	mux.Handle("GET /v1/pulse/profile/{handle}", requireUser(getByHandleHandler(svc)))
	mux.Handle("PATCH /v1/pulse/profile/me", requireUser(updateProfileHandler(svc)))
}

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := pulseauth.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
		return "", false
	}
	return id, true
}

type profileResponse struct {
	UserID      string         `json:"userId"`
	Handle      string         `json:"handle"`
	VisualPrefs map[string]any `json:"visualPrefs,omitempty"`
	PulsePrefs  map[string]any `json:"pulsePrefs,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

func toProfileResponse(p Profile) profileResponse {
	return profileResponse{
		UserID: p.UserID, Handle: p.Handle, VisualPrefs: p.VisualPrefs, PulsePrefs: p.PulsePrefs,
		CreatedAt: p.CreatedAt.UTC().Format(timeFormat), UpdatedAt: p.UpdatedAt.UTC().Format(timeFormat),
	}
}

func ensureProfileHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Handle string `json:"handle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		p, err := svc.EnsureProfile(r.Context(), caller, CreateInput{Handle: body.Handle})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toProfileResponse(p))
	}
}

func getMyProfileHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		p, err := svc.Get(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toProfileResponse(p))
	}
}

func getByHandleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		p, err := svc.GetByHandle(r.Context(), r.PathValue("handle"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toProfileResponse(p))
	}
}

func updateProfileHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			VisualPrefs map[string]any `json:"visualPrefs"`
			PulsePrefs  map[string]any `json:"pulsePrefs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		p, err := svc.Update(r.Context(), caller, UpdateInput{VisualPrefs: body.VisualPrefs, PulsePrefs: body.PulsePrefs})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toProfileResponse(p))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrHandleTaken):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	default:
		slog.Default().Error("unhandled pulseprofile error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
