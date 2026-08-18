package search

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
	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

// RegisterRoutes wires the search endpoints, all requiring an
// authenticated caller. Query is open to any authenticated caller;
// Index/Delete (manual re-indexing) are platform.admin only, enforced in
// Service.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/search", requireUser(queryHandler(svc)))
	mux.Handle("POST /v1/search/index", requireUser(indexHandler(svc)))
	mux.Handle("DELETE /v1/search/index/{type}/{id}", requireUser(deleteHandler(svc)))
}

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

type resultResponse struct {
	Type   string         `json:"type"`
	AppID  string         `json:"appId,omitempty"`
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields,omitempty"`
	Score  float64        `json:"score"`
}

func toResultResponse(r searchidx.Result) resultResponse {
	return resultResponse{Type: r.Document.Type, AppID: r.Document.AppID, ID: r.Document.ID, Fields: r.Document.Fields, Score: r.Score}
}

func queryHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		from, _ := strconv.Atoi(q.Get("from"))
		result, err := svc.Query(r.Context(), QueryInput{
			Type: q.Get("type"), AppID: q.Get("appId"), Query: q.Get("q"), Limit: limit, From: from,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]resultResponse, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, toResultResponse(item))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": result.Total})
	}
}

func indexHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Type   string         `json:"type"`
			AppID  string         `json:"appId"`
			ID     string         `json:"id"`
			Fields map[string]any `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		err := svc.Index(r.Context(), caller, IndexInput{Type: body.Type, AppID: body.AppID, ID: body.ID, Fields: body.Fields})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		err := svc.Delete(r.Context(), caller, r.PathValue("type"), r.URL.Query().Get("appId"), r.PathValue("id"))
		if err != nil {
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
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled search error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
