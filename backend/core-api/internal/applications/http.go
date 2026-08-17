package applications

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the Application Registry's REST surface onto mux.
func RegisterRoutes(mux *http.ServeMux, svc *Service) {
	mux.HandleFunc("POST /v1/apps", createHandler(svc))
	mux.HandleFunc("GET /v1/apps", listHandler(svc))
	mux.HandleFunc("GET /v1/apps/{id}", getHandler(svc))
	mux.HandleFunc("PATCH /v1/apps/{id}", updateHandler(svc))
}

type applicationResponse struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Status    Status `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toResponse(app Application) applicationResponse {
	return applicationResponse{
		ID: app.ID, Slug: app.Slug, Name: app.Name, Status: app.Status,
		CreatedAt: app.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt: app.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func createHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		app, err := svc.Create(r.Context(), CreateInput{Slug: body.Slug, Name: body.Name})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toResponse(app))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(app))
	}
}

func listHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := svc.List(r.Context(), ListParams{
			Limit:  limit,
			Cursor: r.URL.Query().Get("cursor"),
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]applicationResponse, 0, len(result.Items))
		for _, app := range result.Items {
			items = append(items, toResponse(app))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{
			"items":      items,
			"nextCursor": result.NextCursor,
		})
	}
}

func updateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name   *string `json:"name"`
			Status *Status `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		app, err := svc.Update(r.Context(), r.PathValue("id"), UpdateInput{Name: body.Name, Status: body.Status})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(app))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "application not found"))
	case errors.Is(err, ErrSlugTaken):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, "slug already in use"))
	default:
		slog.Default().Error("unhandled applications error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
