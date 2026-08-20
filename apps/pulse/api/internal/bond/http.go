package bond

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
// authenticated client - injected from the composition root
// (internal/api/router.go), never imported directly here, for the same
// import-cycle reason documented in pulseconnections/http.go.
type CoreFactory func(client *coresdk.Client) CoreRelationships

func RegisterRoutes(mux *http.ServeMux, svc *Service, newCore CoreFactory, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/pulse/bond/requests", requireUser(requestHandler(svc, newCore)))
	mux.Handle("POST /v1/pulse/bond/requests/{id}/accept", requireUser(acceptHandler(svc, newCore)))
	mux.Handle("POST /v1/pulse/bond/requests/{id}/decline", requireUser(declineHandler(svc, newCore)))
	mux.Handle("DELETE /v1/pulse/bond/{id}", requireUser(endHandler(svc, newCore)))
	mux.Handle("GET /v1/pulse/bond", requireUser(myActiveBondHandler(svc)))
}

func callerAndCore(w http.ResponseWriter, r *http.Request, newCore CoreFactory) (string, CoreRelationships, bool) {
	callerID, ok := pulseauth.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
		return "", nil, false
	}
	client, ok := pulseauth.ClientFromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "core client missing from context"))
		return "", nil, false
	}
	return callerID, newCore(client), true
}

type bondResponse struct {
	ID          string  `json:"id"`
	OtherUserID string  `json:"otherUserId"`
	Status      string  `json:"status"`
	RequestedAt string  `json:"requestedAt"`
	AcceptedAt  *string `json:"acceptedAt,omitempty"`
	EndedAt     *string `json:"endedAt,omitempty"`
}

func toBondResponse(callerID string, b Bond) bondResponse {
	resp := bondResponse{
		ID: b.ID, OtherUserID: b.otherUser(callerID), Status: string(b.Status),
		RequestedAt: b.RequestedAt.UTC().Format(timeFormat),
	}
	if b.AcceptedAt != nil {
		s := b.AcceptedAt.UTC().Format(timeFormat)
		resp.AcceptedAt = &s
	}
	if b.EndedAt != nil {
		s := b.EndedAt.UTC().Format(timeFormat)
		resp.EndedAt = &s
	}
	return resp
}

func requestHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		var body struct {
			TargetUserID string `json:"targetUserId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		b, err := svc.RequestBond(r.Context(), core, callerID, RequestInput{TargetUserID: body.TargetUserID})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toBondResponse(callerID, b))
	}
}

func acceptHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		b, err := svc.Accept(r.Context(), core, callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toBondResponse(callerID, b))
	}
}

func declineHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		b, err := svc.Decline(r.Context(), core, callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toBondResponse(callerID, b))
	}
}

func endHandler(svc *Service, newCore CoreFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, core, ok := callerAndCore(w, r, newCore)
		if !ok {
			return
		}
		b, err := svc.End(r.Context(), core, callerID, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toBondResponse(callerID, b))
	}
}

func myActiveBondHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		callerID, ok := pulseauth.FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "caller missing from context"))
			return
		}
		b, err := svc.MyActiveBond(r.Context(), callerID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toBondResponse(callerID, b))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	var apiErr *coresdk.APIError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "bond not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrNoConnection):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, err.Error()))
	case errors.Is(err, ErrAlreadyBonded):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.As(err, &apiErr):
		apperr.Write(w, r, apperr.New(apperr.Code(apiErr.Code), apiErr.Message))
	default:
		slog.Default().Error("unhandled bond error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
