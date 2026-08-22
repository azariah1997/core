package moments

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseauth"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// RegisterRoutes takes interactions directly (not a per-caller
// *Factory) - the adapter it's wrapped in needs no per-caller Core
// client, since pulse-interactions.Service.Get reads only Pulse's own
// Postgres and enforces participation itself (see
// internal/moments/pulsemodules).
func RegisterRoutes(mux *http.ServeMux, svc *Service, interactions PulseInteractions, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/moments/{interactionId}/save", requireUser(saveHandler(svc, interactions)))
	mux.Handle("GET /v1/pulse/moments", requireUser(listMineHandler(svc)))
	mux.Handle("DELETE /v1/pulse/moments/{id}", requireUser(deleteHandler(svc)))
}

type momentResponse struct {
	ID              string `json:"id"`
	InteractionID   string `json:"interactionId"`
	OtherUserID     string `json:"otherUserId"`
	InteractionType string `json:"interactionType"`
	DurationMs      *int   `json:"durationMs,omitempty"`
	Pattern         string `json:"pattern,omitempty"`
	OccurredAt      string `json:"occurredAt"`
	SavedAt         string `json:"savedAt"`
}

func toMomentResponse(m Moment) momentResponse {
	resp := momentResponse{
		ID: m.ID, InteractionID: m.InteractionID, OtherUserID: m.OtherUserID, InteractionType: m.InteractionType,
		DurationMs: m.DurationMs, OccurredAt: m.OccurredAt.UTC().Format(timeFormat), SavedAt: m.SavedAt.UTC().Format(timeFormat),
	}
	if m.Pattern != nil {
		resp.Pattern = *m.Pattern
	}
	return resp
}

func saveHandler(svc *Service, interactions PulseInteractions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		m, err := svc.Save(r.Context(), interactions, callerID, r.PathValue("interactionId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMomentResponse(m))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := svc.ListMine(r.Context(), callerID, limit)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]momentResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMomentResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func deleteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		if err := svc.Delete(r.Context(), callerID, r.PathValue("id")); err != nil {
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
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "moment not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNotCompleted):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	default:
		slog.Default().Error("unhandled moments error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
