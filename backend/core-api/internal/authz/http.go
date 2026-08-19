package authz

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

// RegisterRoutes gives authz its first HTTP surface ever - every prior
// phase's live validation that needed to grant a role (moderator,
// platform.admin) did it via a throwaway, not-committed Go program,
// because there was no route to call. Phase 25's Admin Portal needing
// real "Roles" visibility and management is what finally justified
// building one - AssignRole/RevokeRole are platform.admin-only
// unconditionally (never "self," even though RolesFor itself permits
// asking about your own roles - self-granting privileges would defeat
// the entire point of gating this at all).
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/authz/roles", requireUser(listRolesHandler(svc)))
	mux.Handle("POST /v1/authz/roles", requireUser(assignRoleHandler(svc)))
	mux.Handle("POST /v1/authz/roles/revoke", requireUser(revokeRoleHandler(svc)))
}

func callerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	u, ok := users.FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return "", false
	}
	return u.ID, true
}

func listRolesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		targetUserID := r.URL.Query().Get("userId")
		if targetUserID == "" {
			targetUserID = caller
		}
		roles, err := svc.RolesFor(r.Context(), caller, targetUserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"userId": targetUserID, "roles": roles})
	}
}

func requireAdmin(w http.ResponseWriter, r *http.Request, svc *Service, callerID string) bool {
	isAdmin, err := svc.IsPlatformAdmin(r.Context(), callerID)
	if err != nil {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "authorization check failed"))
		return false
	}
	if !isAdmin {
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, "platform.admin required"))
		return false
	}
	return true
}

func assignRoleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if !requireAdmin(w, r, svc, caller) {
			return
		}
		var body struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.UserID == "" || body.Role == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "userId and role are required"))
			return
		}
		if err := svc.AssignRole(r.Context(), caller, body.UserID, Role(body.Role)); err != nil {
			writeDomainError(w, r, err)
			return
		}
		roles, err := svc.RolesFor(r.Context(), caller, body.UserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"userId": body.UserID, "roles": roles})
	}
}

func revokeRoleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		if !requireAdmin(w, r, svc, caller) {
			return
		}
		var body struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		if body.UserID == "" || body.Role == "" {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "userId and role are required"))
			return
		}
		if err := svc.RevokeRole(r.Context(), caller, body.UserID, Role(body.Role)); err != nil {
			writeDomainError(w, r, err)
			return
		}
		roles, err := svc.RolesFor(r.Context(), caller, body.UserID)
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"userId": body.UserID, "roles": roles})
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled authz error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
