package signals

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

// CoreFactory builds a CoreRelationships bound to one caller's
// authenticated client - same import-cycle-avoidance reason every
// other module's *Factory type exists for.
type CoreFactory func(client *coresdk.Client) CoreRelationships

func RegisterRoutes(mux *http.ServeMux, svc *Service, newCore CoreFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/signals", requireUser(createHandler(svc, newCore)))
	mux.Handle("GET /v1/pulse/signals", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/pulse/signals/{id}", requireUser(getHandler(svc)))
	mux.Handle("DELETE /v1/pulse/signals/{id}", requireUser(deleteHandler(svc)))
}

type segmentJSON struct {
	Type       string `json:"type"`
	DurationMs int    `json:"durationMs"`
}

type signalResponse struct {
	ID           string        `json:"id"`
	TargetUserID string        `json:"targetUserId"`
	Label        string        `json:"label,omitempty"`
	Segments     []segmentJSON `json:"segments"`
	CreatedAt    string        `json:"createdAt"`
}

func toSignalResponse(sig Signal) signalResponse {
	segs := make([]segmentJSON, 0, len(sig.Segments))
	for _, s := range sig.Segments {
		segs = append(segs, segmentJSON{Type: string(s.Type), DurationMs: s.DurationMs})
	}
	return signalResponse{
		ID: sig.ID, TargetUserID: sig.TargetUserID, Label: sig.Label, Segments: segs,
		CreatedAt: sig.CreatedAt.UTC().Format(timeFormat),
	}
}

func createHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
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
		var body struct {
			TargetUserID string        `json:"targetUserId"`
			Label        string        `json:"label"`
			Segments     []segmentJSON `json:"segments"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		segs := make([]Segment, 0, len(body.Segments))
		for _, s := range body.Segments {
			segs = append(segs, Segment{Type: SegmentType(s.Type), DurationMs: s.DurationMs})
		}
		sig, err := svc.Create(r.Context(), newCore(client), callerID, CreateInput{TargetUserID: body.TargetUserID, Label: body.Label, Segments: segs})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toSignalResponse(sig))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		list, err := svc.ListMine(r.Context(), callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]signalResponse, 0, len(list))
		for _, sig := range list {
			items = append(items, toSignalResponse(sig))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		sig, err := svc.Get(r.Context(), callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSignalResponse(sig))
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
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "signal not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNotConnected):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrBlocked):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled signals error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
