package trustsafety

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/trustsafety/mutes", requireUser(muteHandler(svc)))
	mux.Handle("GET /v1/trustsafety/mutes", requireUser(listMutesHandler(svc)))
	mux.Handle("DELETE /v1/trustsafety/mutes/{userId}", requireUser(unmuteHandler(svc)))

	mux.Handle("POST /v1/trustsafety/reports", requireUser(createReportHandler(svc)))
	mux.Handle("GET /v1/trustsafety/reports/{id}", requireUser(getReportHandler(svc)))

	mux.Handle("GET /v1/trustsafety/cases", requireUser(listCasesHandler(svc)))
	mux.Handle("GET /v1/trustsafety/cases/{id}", requireUser(getCaseHandler(svc)))
	mux.Handle("GET /v1/trustsafety/cases/{id}/reports", requireUser(listCaseReportsHandler(svc)))
	mux.Handle("PATCH /v1/trustsafety/cases/{id}/assign", requireUser(assignCaseHandler(svc)))
	mux.Handle("PATCH /v1/trustsafety/cases/{id}", requireUser(resolveCaseHandler(svc)))

	mux.Handle("POST /v1/trustsafety/suspensions", requireUser(suspendHandler(svc)))
	mux.Handle("GET /v1/trustsafety/suspensions", requireUser(listSuspensionsHandler(svc)))
	mux.Handle("POST /v1/trustsafety/suspensions/{id}/lift", requireUser(liftSuspensionHandler(svc)))

	mux.Handle("POST /v1/trustsafety/bans", requireUser(banHandler(svc)))
	mux.Handle("GET /v1/trustsafety/bans", requireUser(listBansHandler(svc)))
	mux.Handle("POST /v1/trustsafety/bans/{id}/lift", requireUser(liftBanHandler(svc)))

	mux.Handle("POST /v1/trustsafety/appeals", requireUser(createAppealHandler(svc)))
	mux.Handle("GET /v1/trustsafety/appeals", requireUser(listAppealsHandler(svc)))
	mux.Handle("POST /v1/trustsafety/appeals/{id}/review", requireUser(reviewAppealHandler(svc)))

	mux.Handle("POST /v1/trustsafety/signals", requireUser(recordSignalHandler(svc)))
	mux.Handle("GET /v1/trustsafety/signals", requireUser(listSignalsHandler(svc)))
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

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(timeFormat)
	return &s
}

// --- mutes ---

type muteResponse struct {
	ID          string `json:"id"`
	MutedUserID string `json:"mutedUserId"`
	CreatedAt   string `json:"createdAt"`
}

func toMuteResponse(m Mute) muteResponse {
	return muteResponse{ID: m.ID, MutedUserID: m.MutedUserID, CreatedAt: m.CreatedAt.UTC().Format(timeFormat)}
}

func muteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			MutedUserID string `json:"mutedUserId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.Mute(r.Context(), caller, body.MutedUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMuteResponse(m))
	}
}

func listMutesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMutes(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]muteResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMuteResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func unmuteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.Unmute(r.Context(), caller, r.PathValue("userId")); err != nil {
			writeDomainError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- reports ---

type reportResponse struct {
	ID           string `json:"id"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Reason       string `json:"reason"`
	Details      string `json:"details,omitempty"`
	CaseID       string `json:"caseId"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
}

func toReportResponse(r Report) reportResponse {
	return reportResponse{
		ID: r.ID, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Reason: r.Reason, Details: r.Details,
		CaseID: r.CaseID, Status: string(r.Status), CreatedAt: r.CreatedAt.UTC().Format(timeFormat),
	}
}

func createReportHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			ResourceType string `json:"resourceType"`
			ResourceID   string `json:"resourceId"`
			Reason       string `json:"reason"`
			Details      string `json:"details"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		rep, err := svc.CreateReport(r.Context(), caller, ReportInput{
			ResourceType: body.ResourceType, ResourceID: body.ResourceID, Reason: body.Reason, Details: body.Details,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toReportResponse(rep))
	}
}

func getReportHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		rep, err := svc.GetReport(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toReportResponse(rep))
	}
}

// --- moderation cases ---

type caseResponse struct {
	ID           string  `json:"id"`
	ResourceType string  `json:"resourceType"`
	ResourceID   string  `json:"resourceId"`
	Status       string  `json:"status"`
	AssignedTo   string  `json:"assignedTo,omitempty"`
	Resolution   string  `json:"resolution,omitempty"`
	ReportCount  int     `json:"reportCount"`
	CreatedAt    string  `json:"createdAt"`
	ResolvedAt   *string `json:"resolvedAt,omitempty"`
}

func toCaseResponse(c ModerationCase) caseResponse {
	return caseResponse{
		ID: c.ID, ResourceType: c.ResourceType, ResourceID: c.ResourceID, Status: string(c.Status),
		AssignedTo: c.AssignedTo, Resolution: c.Resolution, ReportCount: c.ReportCount,
		CreatedAt: c.CreatedAt.UTC().Format(timeFormat), ResolvedAt: formatTimePtr(c.ResolvedAt),
	}
}

func listCasesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		status := CaseStatus(r.URL.Query().Get("status"))
		list, err := svc.ListCases(r.Context(), caller, status)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]caseResponse, 0, len(list))
		for _, c := range list {
			items = append(items, toCaseResponse(c))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func getCaseHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		c, err := svc.GetCase(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toCaseResponse(c))
	}
}

func listCaseReportsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListCaseReports(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]reportResponse, 0, len(list))
		for _, rep := range list {
			items = append(items, toReportResponse(rep))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func assignCaseHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AssigneeUserID string `json:"assigneeUserId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		c, err := svc.AssignCase(r.Context(), caller, r.PathValue("id"), body.AssigneeUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toCaseResponse(c))
	}
}

func resolveCaseHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Status     string `json:"status"`
			Resolution string `json:"resolution"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		c, err := svc.ResolveCase(r.Context(), caller, r.PathValue("id"), ResolveCaseInput{Status: CaseStatus(body.Status), Resolution: body.Resolution})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toCaseResponse(c))
	}
}

// --- suspensions ---

type suspensionResponse struct {
	ID       string  `json:"id"`
	UserID   string  `json:"userId"`
	Reason   string  `json:"reason"`
	CaseID   string  `json:"caseId,omitempty"`
	IssuedBy string  `json:"issuedBy"`
	StartsAt string  `json:"startsAt"`
	EndsAt   string  `json:"endsAt"`
	LiftedAt *string `json:"liftedAt,omitempty"`
	Active   bool    `json:"active"`
}

func toSuspensionResponse(s Suspension) suspensionResponse {
	return suspensionResponse{
		ID: s.ID, UserID: s.UserID, Reason: s.Reason, CaseID: s.CaseID, IssuedBy: s.IssuedBy,
		StartsAt: s.StartsAt.UTC().Format(timeFormat), EndsAt: s.EndsAt.UTC().Format(timeFormat),
		LiftedAt: formatTimePtr(s.LiftedAt), Active: s.IsActive(time.Now().UTC()),
	}
}

func suspendHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			UserID          string `json:"userId"`
			Reason          string `json:"reason"`
			CaseID          string `json:"caseId"`
			DurationSeconds int    `json:"durationSeconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		sus, err := svc.Suspend(r.Context(), caller, SuspendInput{
			UserID: body.UserID, Reason: body.Reason, CaseID: body.CaseID, Duration: time.Duration(body.DurationSeconds) * time.Second,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toSuspensionResponse(sus))
	}
}

func listSuspensionsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		list, err := svc.ListSuspensions(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]suspensionResponse, 0, len(list))
		for _, s := range list {
			items = append(items, toSuspensionResponse(s))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func liftSuspensionHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		sus, err := svc.LiftSuspension(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSuspensionResponse(sus))
	}
}

// --- bans ---

type banResponse struct {
	ID        string  `json:"id"`
	UserID    string  `json:"userId"`
	Reason    string  `json:"reason"`
	CaseID    string  `json:"caseId,omitempty"`
	IssuedBy  string  `json:"issuedBy"`
	CreatedAt string  `json:"createdAt"`
	LiftedAt  *string `json:"liftedAt,omitempty"`
	Active    bool    `json:"active"`
}

func toBanResponse(b Ban) banResponse {
	return banResponse{
		ID: b.ID, UserID: b.UserID, Reason: b.Reason, CaseID: b.CaseID, IssuedBy: b.IssuedBy,
		CreatedAt: b.CreatedAt.UTC().Format(timeFormat), LiftedAt: formatTimePtr(b.LiftedAt), Active: b.IsActive(),
	}
}

func banHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			UserID string `json:"userId"`
			Reason string `json:"reason"`
			CaseID string `json:"caseId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		b, err := svc.Ban(r.Context(), caller, BanInput{UserID: body.UserID, Reason: body.Reason, CaseID: body.CaseID})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toBanResponse(b))
	}
}

func listBansHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		list, err := svc.ListBans(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]banResponse, 0, len(list))
		for _, b := range list {
			items = append(items, toBanResponse(b))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func liftBanHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		b, err := svc.LiftBan(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toBanResponse(b))
	}
}

// --- appeals ---

type appealResponse struct {
	ID         string  `json:"id"`
	TargetType string  `json:"targetType"`
	TargetID   string  `json:"targetId"`
	Reason     string  `json:"reason"`
	Status     string  `json:"status"`
	ReviewedBy string  `json:"reviewedBy,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	ReviewedAt *string `json:"reviewedAt,omitempty"`
}

func toAppealResponse(a Appeal) appealResponse {
	return appealResponse{
		ID: a.ID, TargetType: string(a.TargetType), TargetID: a.TargetID, Reason: a.Reason, Status: string(a.Status),
		ReviewedBy: a.ReviewedBy, CreatedAt: a.CreatedAt.UTC().Format(timeFormat), ReviewedAt: formatTimePtr(a.ReviewedAt),
	}
}

func createAppealHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			TargetType string `json:"targetType"`
			TargetID   string `json:"targetId"`
			Reason     string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		a, err := svc.CreateAppeal(r.Context(), caller, AppealInput{
			TargetType: AppealTargetType(body.TargetType), TargetID: body.TargetID, Reason: body.Reason,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toAppealResponse(a))
	}
}

func listAppealsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		list, err := svc.ListAppeals(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]appealResponse, 0, len(list))
		for _, a := range list {
			items = append(items, toAppealResponse(a))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func reviewAppealHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Approve bool `json:"approve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		a, err := svc.ReviewAppeal(r.Context(), caller, r.PathValue("id"), body.Approve)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toAppealResponse(a))
	}
}

// --- abuse signals ---

type signalResponse struct {
	ID           string         `json:"id"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	SignalType   string         `json:"signalType"`
	Severity     string         `json:"severity"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	RecordedAt   string         `json:"recordedAt"`
}

func toSignalResponse(s AbuseSignal) signalResponse {
	return signalResponse{
		ID: s.ID, ResourceType: s.ResourceType, ResourceID: s.ResourceID, SignalType: s.SignalType,
		Severity: s.Severity, Metadata: s.Metadata, RecordedAt: s.RecordedAt.UTC().Format(timeFormat),
	}
}

func recordSignalHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := callerID(w, r); !ok {
			return
		}
		var body struct {
			ResourceType string         `json:"resourceType"`
			ResourceID   string         `json:"resourceId"`
			SignalType   string         `json:"signalType"`
			Severity     string         `json:"severity"`
			Metadata     map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		sig, err := svc.RecordSignal(r.Context(), SignalInput{
			ResourceType: body.ResourceType, ResourceID: body.ResourceID, SignalType: body.SignalType,
			Severity: SignalSeverity(body.Severity), Metadata: body.Metadata,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toSignalResponse(sig))
	}
}

func listSignalsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListSignals(r.Context(), caller, r.URL.Query().Get("resourceType"), r.URL.Query().Get("resourceId"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]signalResponse, 0, len(list))
		for _, s := range list {
			items = append(items, toSignalResponse(s))
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
	case errors.Is(err, ErrAlreadyReviewed):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrRateLimited):
		apperr.Write(w, r, apperr.New(apperr.CodeRateLimited, err.Error()))
	default:
		slog.Default().Error("unhandled trustsafety error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
