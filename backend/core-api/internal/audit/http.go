package audit

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the audit endpoints - deliberately only three
// routes. There is no PATCH or DELETE route because there is no
// corresponding Service method to call: immutability enforced by
// omission, not a permission check that could be misconfigured.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/audit", requireUser(recordHandler(svc)))
	mux.Handle("GET /v1/audit", requireUser(listHandler(svc)))
	mux.Handle("GET /v1/audit/{id}", requireUser(getHandler(svc)))
}

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

type recordResponse struct {
	ID            string         `json:"id"`
	ActorUserID   string         `json:"actorUserId,omitempty"`
	Action        string         `json:"action"`
	ResourceType  string         `json:"resourceType"`
	ResourceID    string         `json:"resourceId"`
	AppID         string         `json:"appId,omitempty"`
	TenantID      string         `json:"tenantId,omitempty"`
	DeviceID      string         `json:"deviceId,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	OccurredAt    string         `json:"occurredAt"`
}

func toRecordResponse(r Record) recordResponse {
	return recordResponse{
		ID: r.ID, ActorUserID: r.ActorUserID, Action: r.Action, ResourceType: r.ResourceType, ResourceID: r.ResourceID,
		AppID: r.AppID, TenantID: r.TenantID, DeviceID: r.DeviceID, CorrelationID: r.CorrelationID, Metadata: r.Metadata,
		OccurredAt: r.OccurredAt.UTC().Format(timeFormat),
	}
}

func recordHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Action       string         `json:"action"`
			ResourceType string         `json:"resourceType"`
			ResourceID   string         `json:"resourceId"`
			AppID        string         `json:"appId"`
			TenantID     string         `json:"tenantId"`
			DeviceID     string         `json:"deviceId"`
			Metadata     map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		// ActorUserID is always the authenticated caller, never taken from
		// the request body - an audit record can never be submitted
		// claiming to be someone else's action.
		record, err := svc.Record(r.Context(), RecordInput{
			ActorUserID: caller, Action: body.Action, ResourceType: body.ResourceType, ResourceID: body.ResourceID,
			AppID: body.AppID, TenantID: body.TenantID, DeviceID: body.DeviceID, Metadata: body.Metadata,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toRecordResponse(record))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		record, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toRecordResponse(record))
	}
}

func listHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		result, err := svc.List(r.Context(), caller, ListFilter{
			AppID: q.Get("appId"), ActorUserID: q.Get("actorUserId"), ResourceType: q.Get("resourceType"),
			ResourceID: q.Get("resourceId"), Action: q.Get("action"), Limit: limit, Cursor: q.Get("cursor"),
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]recordResponse, 0, len(result.Items))
		for _, rec := range result.Items {
			items = append(items, toRecordResponse(rec))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled audit error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
