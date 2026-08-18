package relationships

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the relationship endpoints, all requiring an
// authenticated caller. Per-relationship permission checks (only the
// target may accept/decline, only a participant may remove/block through
// an existing row) happen inside Service.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/relationships", requireUser(requestHandler(svc)))
	mux.Handle("GET /v1/relationships", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/relationships/{id}", requireUser(getHandler(svc)))
	mux.Handle("POST /v1/relationships/{id}/accept", requireUser(acceptHandler(svc)))
	mux.Handle("POST /v1/relationships/{id}/decline", requireUser(declineHandler(svc)))
	mux.Handle("DELETE /v1/relationships/{id}", requireUser(removeHandler(svc)))
	mux.Handle("POST /v1/relationships/block", requireUser(blockHandler(svc)))
}

type relationshipResponse struct {
	ID          string         `json:"id"`
	AppID       string         `json:"appId"`
	RequesterID string         `json:"requesterUserId"`
	TargetID    string         `json:"targetUserId"`
	Type        string         `json:"type"`
	Status      Status         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

func toResponse(r Relationship) relationshipResponse {
	return relationshipResponse{
		ID: r.ID, AppID: r.AppID, RequesterID: r.RequesterID, TargetID: r.TargetID,
		Type: r.Type, Status: r.Status, Metadata: r.Metadata,
		CreatedAt: r.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt: r.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

func requestHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID        string         `json:"appId"`
			TargetUserID string         `json:"targetUserId"`
			Type         string         `json:"type"`
			Metadata     map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		rel, err := svc.Request(r.Context(), RequestInput{
			AppID: body.AppID, RequesterID: caller, TargetID: body.TargetUserID,
			Type: body.Type, Metadata: body.Metadata,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toResponse(rel))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		filter := ListFilter{
			Type:   r.URL.Query().Get("type"),
			Status: Status(r.URL.Query().Get("status")),
		}
		list, err := svc.ListMine(r.Context(), r.URL.Query().Get("appId"), caller, filter)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]relationshipResponse, 0, len(list))
		for _, rel := range list {
			items = append(items, toResponse(rel))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		rel, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(rel))
	}
}

func acceptHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		rel, err := svc.Accept(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(rel))
	}
}

func declineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		rel, err := svc.Decline(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(rel))
	}
}

func removeHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if _, err := svc.Remove(r.Context(), caller, r.PathValue("id")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func blockHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID        string `json:"appId"`
			TargetUserID string `json:"targetUserId"`
			Type         string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		rel, err := svc.Block(r.Context(), caller, body.AppID, body.TargetUserID, body.Type)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(rel))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "relationship not found"))
	case errors.Is(err, ErrExists), errors.Is(err, ErrInvalidTransition):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled relationships error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
