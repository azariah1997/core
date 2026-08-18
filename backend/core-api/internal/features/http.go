package features

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

// RegisterRoutes wires the feature-flag endpoints, all requiring an
// authenticated caller. Management (create/update/rules) is
// platform.admin only, enforced in Service; reads and Evaluate are open
// to any authenticated caller.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/features", requireUser(createFeatureHandler(svc)))
	mux.Handle("GET /v1/features", requireUser(listFeaturesHandler(svc)))
	mux.Handle("GET /v1/features/{appId}/{key}", requireUser(getFeatureHandler(svc)))
	mux.Handle("PATCH /v1/features/{appId}/{key}", requireUser(updateFeatureHandler(svc)))
	mux.Handle("POST /v1/features/{appId}/{key}/evaluate", requireUser(evaluateHandler(svc)))
	mux.Handle("POST /v1/features/{appId}/{key}/rules", requireUser(createRuleHandler(svc)))
	mux.Handle("GET /v1/features/{appId}/{key}/rules", requireUser(listRulesHandler(svc)))
	mux.Handle("PATCH /v1/features/{appId}/{key}/rules/{ruleId}", requireUser(updateRuleHandler(svc)))
	mux.Handle("DELETE /v1/features/{appId}/{key}/rules/{ruleId}", requireUser(deleteRuleHandler(svc)))
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

type featureResponse struct {
	ID          string `json:"id"`
	AppID       string `json:"appId"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func toFeatureResponse(f Feature) featureResponse {
	return featureResponse{
		ID: f.ID, AppID: f.AppID, Key: f.Key, Name: f.Name, Description: f.Description, Enabled: f.Enabled,
		CreatedAt: f.CreatedAt.UTC().Format(timeFormat), UpdatedAt: f.UpdatedAt.UTC().Format(timeFormat),
	}
}

type ruleResponse struct {
	ID         string         `json:"id"`
	FeatureID  string         `json:"featureId"`
	Priority   int            `json:"priority"`
	Conditions RuleConditions `json:"conditions"`
	Enabled    bool           `json:"enabled"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
}

func toRuleResponse(r FeatureRule) ruleResponse {
	return ruleResponse{
		ID: r.ID, FeatureID: r.FeatureID, Priority: r.Priority, Conditions: r.Conditions, Enabled: r.Enabled,
		CreatedAt: r.CreatedAt.UTC().Format(timeFormat), UpdatedAt: r.UpdatedAt.UTC().Format(timeFormat),
	}
}

type evaluationResponse struct {
	FeatureKey    string `json:"featureKey"`
	Enabled       bool   `json:"enabled"`
	Reason        string `json:"reason"`
	MatchedRuleID string `json:"matchedRuleId,omitempty"`
}

func toEvaluationResponse(e FeatureEvaluation) evaluationResponse {
	return evaluationResponse{FeatureKey: e.FeatureKey, Enabled: e.Enabled, Reason: e.Reason, MatchedRuleID: e.MatchedRuleID}
}

func createFeatureHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID       string `json:"appId"`
			Key         string `json:"key"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		f, err := svc.CreateFeature(r.Context(), caller, CreateFeatureInput{
			AppID: body.AppID, Key: body.Key, Name: body.Name, Description: body.Description, Enabled: body.Enabled,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toFeatureResponse(f))
	}
}

func listFeaturesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		appID := r.URL.Query().Get("appId")
		if appID == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "appId query parameter is required"))
			return
		}
		list, err := svc.ListFeatures(r.Context(), appID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]featureResponse, 0, len(list))
		for _, f := range list {
			items = append(items, toFeatureResponse(f))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getFeatureHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		f, err := svc.GetFeature(r.Context(), r.PathValue("appId"), r.PathValue("key"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toFeatureResponse(f))
	}
}

func updateFeatureHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
			Enabled     *bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		f, err := svc.UpdateFeature(r.Context(), caller, r.PathValue("appId"), r.PathValue("key"),
			UpdateFeatureInput{Name: body.Name, Description: body.Description, Enabled: body.Enabled})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toFeatureResponse(f))
	}
}

func evaluateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		var body struct {
			Environment string `json:"environment"`
			UserID      string `json:"userId"`
			TenantID    string `json:"tenantId"`
			Platform    string `json:"platform"`
			Country     string `json:"country"`
			Version     string `json:"version"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
				return
			}
		}
		eval, err := svc.Evaluate(r.Context(), r.PathValue("appId"), r.PathValue("key"), EvaluationContext{
			Environment: body.Environment, UserID: body.UserID, TenantID: body.TenantID,
			Platform: body.Platform, Country: body.Country, Version: body.Version,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toEvaluationResponse(eval))
	}
}

func createRuleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Priority   int            `json:"priority"`
			Conditions RuleConditions `json:"conditions"`
			Enabled    bool           `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		rule, err := svc.CreateRule(r.Context(), caller, r.PathValue("appId"), r.PathValue("key"),
			CreateRuleInput{Priority: body.Priority, Conditions: body.Conditions, Enabled: body.Enabled})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toRuleResponse(rule))
	}
}

func listRulesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		list, err := svc.ListRules(r.Context(), r.PathValue("appId"), r.PathValue("key"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]ruleResponse, 0, len(list))
		for _, rule := range list {
			items = append(items, toRuleResponse(rule))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func updateRuleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Priority   *int            `json:"priority"`
			Conditions *RuleConditions `json:"conditions"`
			Enabled    *bool           `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		rule, err := svc.UpdateRule(r.Context(), caller, r.PathValue("appId"), r.PathValue("key"), r.PathValue("ruleId"),
			UpdateRuleInput{Priority: body.Priority, Conditions: body.Conditions, Enabled: body.Enabled})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toRuleResponse(rule))
	}
}

func deleteRuleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteRule(r.Context(), caller, r.PathValue("appId"), r.PathValue("key"), r.PathValue("ruleId")); err != nil {
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
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrRuleNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrKeyTaken):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled features error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
