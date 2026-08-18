package jobs

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

// RegisterRoutes wires the job endpoints, all requiring an authenticated
// caller. Ownership checks happen inside Service, not here.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/jobs", requireUser(enqueueHandler(svc)))
	mux.Handle("GET /v1/jobs", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/jobs/{id}", requireUser(getHandler(svc)))
	mux.Handle("GET /v1/jobs/{id}/attempts", requireUser(listAttemptsHandler(svc)))
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

type jobResponse struct {
	ID                        string         `json:"id"`
	AppID                     string         `json:"appId,omitempty"`
	Type                      string         `json:"type"`
	Payload                   map[string]any `json:"payload,omitempty"`
	Status                    Status         `json:"status"`
	RunAt                     string         `json:"runAt"`
	RecurrenceIntervalSeconds *int           `json:"recurrenceIntervalSeconds,omitempty"`
	MaxAttempts               int            `json:"maxAttempts"`
	Attempts                  int            `json:"attempts"`
	CreatedAt                 string         `json:"createdAt"`
	UpdatedAt                 string         `json:"updatedAt"`
}

func toJobResponse(j Job) jobResponse {
	resp := jobResponse{
		ID: j.ID, AppID: j.AppID, Type: j.Type, Payload: j.Payload, Status: j.Status,
		RunAt: j.RunAt.UTC().Format(timeFormat), MaxAttempts: j.MaxAttempts, Attempts: j.Attempts,
		CreatedAt: j.CreatedAt.UTC().Format(timeFormat), UpdatedAt: j.UpdatedAt.UTC().Format(timeFormat),
	}
	if j.RecurrenceInterval != nil {
		seconds := int(j.RecurrenceInterval.Seconds())
		resp.RecurrenceIntervalSeconds = &seconds
	}
	return resp
}

type attemptResponse struct {
	ID            string `json:"id"`
	JobID         string `json:"jobId"`
	AttemptNumber int    `json:"attemptNumber"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
}

func toAttemptResponse(a JobAttempt) attemptResponse {
	return attemptResponse{
		ID: a.ID, JobID: a.JobID, AttemptNumber: a.AttemptNumber, Status: a.Status, Error: a.Error,
		StartedAt: a.StartedAt.UTC().Format(timeFormat), FinishedAt: a.FinishedAt.UTC().Format(timeFormat),
	}
}

func enqueueHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID                     string         `json:"appId"`
			Type                      string         `json:"type"`
			Payload                   map[string]any `json:"payload"`
			RunAt                     *time.Time     `json:"runAt"`
			DelaySeconds              *int           `json:"delaySeconds"`
			RecurrenceIntervalSeconds *int           `json:"recurrenceIntervalSeconds"`
			MaxAttempts               *int           `json:"maxAttempts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		j, err := svc.Enqueue(r.Context(), caller, EnqueueInput{
			AppID: body.AppID, Type: body.Type, Payload: body.Payload, RunAt: body.RunAt,
			DelaySeconds: body.DelaySeconds, RecurrenceIntervalSeconds: body.RecurrenceIntervalSeconds, MaxAttempts: body.MaxAttempts,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toJobResponse(j))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := svc.ListMine(r.Context(), caller, ListParams{Limit: limit, Cursor: r.URL.Query().Get("cursor")})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]jobResponse, 0, len(result.Items))
		for _, j := range result.Items {
			items = append(items, toJobResponse(j))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		j, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toJobResponse(j))
	}
}

func listAttemptsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListAttempts(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]attemptResponse, 0, len(list))
		for _, a := range list {
			items = append(items, toAttemptResponse(a))
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
		slog.Default().Error("unhandled jobs error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
