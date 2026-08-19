package aigateway

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

func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/ai/completions", requireUser(completeHandler(svc)))
	mux.Handle("GET /v1/ai/usage", requireUser(listUsageHandler(svc)))
	mux.Handle("GET /v1/ai/models", requireUser(listModelsHandler(svc)))
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

type messageBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionResponse struct {
	ID               string  `json:"id"`
	ModelAlias       string  `json:"modelAlias"`
	Provider         string  `json:"provider,omitempty"`
	Model            string  `json:"model,omitempty"`
	PromptKey        string  `json:"promptKey,omitempty"`
	PromptVersion    string  `json:"promptVersion,omitempty"`
	Text             string  `json:"text,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	CostCents        float64 `json:"costCents"`
	LatencyMS        int64   `json:"latencyMs"`
	FinishReason     string  `json:"finishReason,omitempty"`
	Status           string  `json:"status"`
	Error            string  `json:"error,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

func toCompletionResponse(c Completion, text string) completionResponse {
	return completionResponse{
		ID: c.ID, ModelAlias: c.ModelAlias, Provider: c.Provider, Model: c.Model, PromptKey: c.PromptKey,
		PromptVersion: c.PromptVersion, Text: text, PromptTokens: c.PromptTokens, CompletionTokens: c.CompletionTokens,
		TotalTokens: c.TotalTokens, CostCents: c.CostCents, LatencyMS: c.LatencyMS, FinishReason: c.FinishReason,
		Status: string(c.Status), Error: c.Error, CreatedAt: c.CreatedAt.UTC().Format(timeFormat),
	}
}

func completeHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			ModelAlias    string        `json:"modelAlias"`
			Messages      []messageBody `json:"messages"`
			MaxTokens     int           `json:"maxTokens"`
			Temperature   float64       `json:"temperature"`
			AppID         string        `json:"appId"`
			PromptKey     string        `json:"promptKey"`
			PromptVersion string        `json:"promptVersion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		messages := make([]Message, 0, len(body.Messages))
		for _, m := range body.Messages {
			messages = append(messages, Message{Role: m.Role, Content: m.Content})
		}
		c, err := svc.Complete(r.Context(), caller, CompletionInput{
			ModelAlias: body.ModelAlias, Messages: messages, MaxTokens: body.MaxTokens, Temperature: body.Temperature,
			AppID: body.AppID, PromptKey: body.PromptKey, PromptVersion: body.PromptVersion,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toCompletionResponse(c, c.Text))
	}
}

func listUsageHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := svc.ListCompletions(r.Context(), caller, targetUserID, limit)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]completionResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toCompletionResponse(c, ""))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

type routeResponse struct {
	Alias string     `json:"alias"`
	Steps []stepBody `json:"steps"`
}

type stepBody struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func listModelsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		routes := svc.Routes()
		items := make([]routeResponse, 0, len(routes))
		for _, route := range routes {
			steps := make([]stepBody, 0, len(route.Steps))
			for _, s := range route.Steps {
				steps = append(steps, stepBody{Provider: s.Provider, Model: s.Model})
			}
			items = append(items, routeResponse{Alias: route.Alias, Steps: steps})
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	case errors.Is(err, ErrUnknownRoute):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, err.Error()))
	case errors.Is(err, ErrRateLimited):
		apperr.Write(w, r, apperr.New(apperr.CodeRateLimited, err.Error()))
	case errors.Is(err, ErrAllStepsFailed):
		apperr.Write(w, r, apperr.New(apperr.CodeDependency, err.Error()))
	default:
		slog.Default().Error("unhandled aigateway error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
