package remoteconfig

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

// RegisterRoutes wires the remote-config endpoints, all requiring an
// authenticated caller. Writes and history are platform.admin only,
// enforced in Service; Get/List are open to any authenticated caller.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/config", requireUser(listHandler(svc)))
	mux.Handle("PUT /v1/config/{appId}/{environment}/{key}", requireUser(setHandler(svc)))
	mux.Handle("GET /v1/config/{appId}/{environment}/{key}", requireUser(getHandler(svc)))
	mux.Handle("DELETE /v1/config/{appId}/{environment}/{key}", requireUser(deleteHandler(svc)))
	mux.Handle("GET /v1/config/{appId}/{environment}/{key}/history", requireUser(historyHandler(svc)))
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

type entryResponse struct {
	ID          string `json:"id"`
	AppID       string `json:"appId"`
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Value       any    `json:"value"`
	Description string `json:"description,omitempty"`
	UpdatedBy   string `json:"updatedBy,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func toEntryResponse(e Entry) entryResponse {
	return entryResponse{
		ID: e.ID, AppID: e.AppID, Environment: e.Environment, Key: e.Key, Value: e.Value,
		Description: e.Description, UpdatedBy: e.UpdatedBy,
		CreatedAt: e.CreatedAt.UTC().Format(timeFormat), UpdatedAt: e.UpdatedAt.UTC().Format(timeFormat),
	}
}

type changeResponse struct {
	ID            string `json:"id"`
	AppID         string `json:"appId"`
	Environment   string `json:"environment"`
	Key           string `json:"key"`
	PreviousValue any    `json:"previousValue,omitempty"`
	NewValue      any    `json:"newValue,omitempty"`
	ChangedBy     string `json:"changedBy,omitempty"`
	Reason        string `json:"reason,omitempty"`
	ChangedAt     string `json:"changedAt"`
}

func toChangeResponse(c Change) changeResponse {
	return changeResponse{
		ID: c.ID, AppID: c.AppID, Environment: c.Environment, Key: c.Key,
		PreviousValue: c.PreviousValue, NewValue: c.NewValue, ChangedBy: c.ChangedBy, Reason: c.Reason,
		ChangedAt: c.ChangedAt.UTC().Format(timeFormat),
	}
}

func setHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Value       any    `json:"value"`
			Description string `json:"description"`
			Reason      string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		entry, err := svc.Set(r.Context(), caller, SetInput{
			AppID: r.PathValue("appId"), Environment: r.PathValue("environment"), Key: r.PathValue("key"),
			Value: body.Value, Description: body.Description, Reason: body.Reason,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toEntryResponse(entry))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		entry, err := svc.Get(r.Context(), r.PathValue("appId"), r.PathValue("environment"), r.PathValue("key"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toEntryResponse(entry))
	}
}

func listHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		appID := r.URL.Query().Get("appId")
		environment := r.URL.Query().Get("environment")
		if appID == "" || environment == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId and environment query parameters are required"))
			return
		}
		list, err := svc.List(r.Context(), appID, environment)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]entryResponse, 0, len(list))
		for _, e := range list {
			items = append(items, toEntryResponse(e))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func deleteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Reason string `json:"reason"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
				return
			}
		}
		err := svc.Delete(r.Context(), caller, r.PathValue("appId"), r.PathValue("environment"), r.PathValue("key"), body.Reason)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func historyHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.History(r.Context(), caller, r.PathValue("appId"), r.PathValue("environment"), r.PathValue("key"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]changeResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toChangeResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
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
		slog.Default().Error("unhandled remoteconfig error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
