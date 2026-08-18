package tenants

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

// RegisterRoutes wires the tenant/membership endpoints, all requiring an
// authenticated caller (requireUser, built in internal/api). Per-tenant
// membership authorization happens inside Service, not here.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/tenants", requireUser(createHandler(svc)))
	mux.Handle("GET /v1/tenants", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/tenants/{id}", requireUser(getHandler(svc)))
	mux.Handle("PATCH /v1/tenants/{id}", requireUser(updateHandler(svc)))
	mux.Handle("GET /v1/tenants/{id}/members", requireUser(listMembersHandler(svc)))
	mux.Handle("POST /v1/tenants/{id}/members", requireUser(addMemberHandler(svc)))
	mux.Handle("DELETE /v1/tenants/{id}/members/{userId}", requireUser(removeMemberHandler(svc)))
}

type tenantResponse struct {
	ID        string `json:"id"`
	AppID     string `json:"appId"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Status    Status `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toTenantResponse(t Tenant) tenantResponse {
	return tenantResponse{
		ID: t.ID, AppID: t.AppID, Slug: t.Slug, Name: t.Name, Status: t.Status,
		CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt: t.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

type membershipResponse struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenantId"`
	UserID    string         `json:"userId"`
	Role      MembershipRole `json:"role"`
	CreatedAt string         `json:"createdAt"`
}

func toMembershipResponse(m Membership) membershipResponse {
	return membershipResponse{
		ID: m.ID, TenantID: m.TenantID, UserID: m.UserID, Role: m.Role,
		CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
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

func createHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			AppID string `json:"appId"`
			Slug  string `json:"slug"`
			Name  string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		t, err := svc.Create(r.Context(), caller, CreateInput{AppID: body.AppID, Slug: body.Slug, Name: body.Name})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toTenantResponse(t))
	}
}

func listMineHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMine(r.Context(), caller)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]tenantResponse, 0, len(list))
		for _, t := range list {
			items = append(items, toTenantResponse(t))
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
		t, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toTenantResponse(t))
	}
}

func updateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Name   *string `json:"name"`
			Status *Status `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		t, err := svc.Update(r.Context(), caller, r.PathValue("id"), UpdateInput{Name: body.Name, Status: body.Status})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toTenantResponse(t))
	}
}

func listMembersHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListMembers(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		items := make([]membershipResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMembershipResponse(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
	}
}

func addMemberHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			UserID string         `json:"userId"`
			Role   MembershipRole `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.AddMember(r.Context(), caller, r.PathValue("id"), AddMemberInput{UserID: body.UserID, Role: body.Role})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMembershipResponse(m))
	}
}

func removeMemberHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if err := svc.RemoveMember(r.Context(), caller, r.PathValue("id"), r.PathValue("userId")); err != nil {
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
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrMembershipNotFound):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "not found"))
	case errors.Is(err, ErrSlugTaken), errors.Is(err, ErrAlreadyMember):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrManagerRequired):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled tenants error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
