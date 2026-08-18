package workflows

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

// RegisterRoutes wires the workflow endpoints, all requiring an
// authenticated caller. Ownership checks happen inside Service, not here.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/workflows", requireUser(startHandler(svc)))
	mux.Handle("GET /v1/workflows/{id}", requireUser(getHandler(svc)))
	mux.Handle("POST /v1/workflows/{id}/signal", requireUser(signalHandler(svc)))
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

type workflowResponse struct {
	WorkflowID string         `json:"workflowId"`
	RunID      string         `json:"runId"`
	Type       string         `json:"type"`
	CreatedAt  string         `json:"createdAt"`
	Status     Status         `json:"status,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func toWorkflowResponse(run WorkflowRun) workflowResponse {
	return workflowResponse{
		WorkflowID: run.WorkflowID, RunID: run.RunID, Type: run.Type, CreatedAt: run.CreatedAt.UTC().Format(timeFormat),
	}
}

func startHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Type         string         `json:"type"`
			Payload      map[string]any `json:"payload"`
			CronSchedule string         `json:"cronSchedule"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		run, err := svc.Start(r.Context(), caller, StartInput{Type: body.Type, Payload: body.Payload, CronSchedule: body.CronSchedule})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toWorkflowResponse(run))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		run, exec, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		resp := toWorkflowResponse(run)
		resp.Status, resp.Result, resp.Error = exec.Status, exec.Result, exec.Error
		httpx.JSON(w, http.StatusOK, resp)
	}
}

func signalHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Name    string         `json:"name"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if err := svc.Signal(r.Context(), caller, r.PathValue("id"), SignalInput{Name: body.Name, Payload: body.Payload}); err != nil {
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
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled workflows error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
