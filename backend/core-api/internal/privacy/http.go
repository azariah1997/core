package privacy

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

func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/privacy/consent", requireUser(setConsentHandler(svc)))
	mux.Handle("GET /v1/privacy/consent", requireUser(listConsentHandler(svc)))
	mux.Handle("GET /v1/privacy/preferences", requireUser(getPreferencesHandler(svc)))
	mux.Handle("PUT /v1/privacy/preferences", requireUser(setPreferencesHandler(svc)))
	mux.Handle("POST /v1/privacy/retention-policies", requireUser(createRetentionPolicyHandler(svc)))
	mux.Handle("GET /v1/privacy/retention-policies", requireUser(listRetentionPoliciesHandler(svc)))
	mux.Handle("POST /v1/privacy/export", requireUser(requestExportHandler(svc)))
	mux.Handle("GET /v1/privacy/export/{id}", requireUser(getExportHandler(svc)))
	mux.Handle("GET /v1/privacy/export/{id}/download", requireUser(downloadExportHandler(svc)))
	mux.Handle("POST /v1/privacy/delete", requireUser(requestDeletionHandler(svc)))
	mux.Handle("GET /v1/privacy/delete/{id}", requireUser(getDeletionHandler(svc)))
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

type consentResponse struct {
	ID         string `json:"id"`
	Purpose    string `json:"purpose"`
	Granted    bool   `json:"granted"`
	Version    string `json:"version"`
	RecordedAt string `json:"recordedAt"`
}

func toConsentResponse(c Consent) consentResponse {
	return consentResponse{ID: c.ID, Purpose: c.Purpose, Granted: c.Granted, Version: c.Version, RecordedAt: c.RecordedAt.UTC().Format(timeFormat)}
}

func setConsentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Purpose string `json:"purpose"`
			Granted bool   `json:"granted"`
			Version string `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		c, err := svc.SetConsent(r.Context(), caller, ConsentInput{Purpose: body.Purpose, Granted: body.Granted, Version: body.Version})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toConsentResponse(c))
	}
}

func listConsentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListConsent(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]consentResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toConsentResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getPreferencesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		prefs, err := svc.GetPreferences(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"preferences": prefs})
	}
}

func setPreferencesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Preferences map[string]bool `json:"preferences"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		prefs, err := svc.SetPreferences(r.Context(), caller, body.Preferences)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"preferences": prefs})
	}
}

type retentionPolicyResponse struct {
	ID            string `json:"id"`
	AppID         string `json:"appId,omitempty"`
	ResourceType  string `json:"resourceType"`
	RetentionDays int    `json:"retentionDays"`
	CreatedBy     string `json:"createdBy,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func toRetentionPolicyResponse(p RetentionPolicy) retentionPolicyResponse {
	return retentionPolicyResponse{
		ID: p.ID, AppID: p.AppID, ResourceType: p.ResourceType, RetentionDays: p.RetentionDays, CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt.UTC().Format(timeFormat), UpdatedAt: p.UpdatedAt.UTC().Format(timeFormat),
	}
}

func createRetentionPolicyHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID         string `json:"appId"`
			ResourceType  string `json:"resourceType"`
			RetentionDays int    `json:"retentionDays"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		p, err := svc.CreateRetentionPolicy(r.Context(), caller, RetentionPolicyInput{AppID: body.AppID, ResourceType: body.ResourceType, RetentionDays: body.RetentionDays})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toRetentionPolicyResponse(p))
	}
}

func listRetentionPoliciesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListRetentionPolicies(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]retentionPolicyResponse, 0, len(list))
		for _, p := range list {
			items = append(items, toRetentionPolicyResponse(p))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

type exportResponse struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Error       string  `json:"error,omitempty"`
	RequestedAt string  `json:"requestedAt"`
	CompletedAt *string `json:"completedAt,omitempty"`
}

func toExportResponse(r ExportRequest) exportResponse {
	resp := exportResponse{ID: r.ID, Status: string(r.Status), Error: r.Error, RequestedAt: r.RequestedAt.UTC().Format(timeFormat)}
	if r.CompletedAt != nil {
		s := r.CompletedAt.UTC().Format(timeFormat)
		resp.CompletedAt = &s
	}
	return resp
}

func requestExportHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		req, err := svc.RequestExport(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusAccepted, toExportResponse(req))
	}
}

func getExportHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		req, err := svc.GetExportRequest(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toExportResponse(req))
	}
}

func downloadExportHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		url, err := svc.GetExportDownloadURL(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"downloadUrl": url})
	}
}

type deletionResponse struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	Results     map[string]any `json:"results,omitempty"`
	Error       string         `json:"error,omitempty"`
	RequestedAt string         `json:"requestedAt"`
	CompletedAt *string        `json:"completedAt,omitempty"`
}

func toDeletionResponse(r DeletionRequest) deletionResponse {
	resp := deletionResponse{ID: r.ID, Status: string(r.Status), Results: r.Results, Error: r.Error, RequestedAt: r.RequestedAt.UTC().Format(timeFormat)}
	if r.CompletedAt != nil {
		s := r.CompletedAt.UTC().Format(timeFormat)
		resp.CompletedAt = &s
	}
	return resp
}

func requestDeletionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		req, err := svc.RequestDeletion(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusAccepted, toDeletionResponse(req))
	}
}

func getDeletionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		req, err := svc.GetDeletionRequest(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toDeletionResponse(req))
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
	case errors.Is(err, ErrNotReady):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, err.Error()))
	default:
		slog.Default().Error("unhandled privacy error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
