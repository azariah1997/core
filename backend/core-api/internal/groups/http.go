package groups

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

// RegisterRoutes wires the group/membership endpoints, all requiring an
// authenticated caller. Per-group membership authorization happens inside
// Service, not here.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser func(http.Handler) http.Handler) {
	mux.Handle("POST /v1/groups", requireUser(createHandler(svc)))
	mux.Handle("GET /v1/groups", requireUser(listMineHandler(svc)))
	mux.Handle("GET /v1/groups/{id}", requireUser(getHandler(svc)))
	mux.Handle("PATCH /v1/groups/{id}", requireUser(updateHandler(svc)))
	mux.Handle("GET /v1/groups/{id}/members", requireUser(listMembersHandler(svc)))
	mux.Handle("POST /v1/groups/{id}/members", requireUser(addMemberHandler(svc)))
	mux.Handle("PATCH /v1/groups/{id}/members/{userId}", requireUser(updateMemberHandler(svc)))
	mux.Handle("DELETE /v1/groups/{id}/members/{userId}", requireUser(removeMemberHandler(svc)))
}

type groupResponse struct {
	ID        string         `json:"id"`
	AppID     string         `json:"appId"`
	Name      string         `json:"name"`
	Status    Status         `json:"status"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

func toGroupResponse(g Group) groupResponse {
	return groupResponse{
		ID: g.ID, AppID: g.AppID, Name: g.Name, Status: g.Status, Metadata: g.Metadata,
		CreatedAt: g.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt: g.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

type memberResponse struct {
	ID        string `json:"id"`
	GroupID   string `json:"groupId"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	IsManager bool   `json:"isManager"`
	CreatedAt string `json:"createdAt"`
}

func toMemberResponse(m GroupMember) memberResponse {
	return memberResponse{
		ID: m.ID, GroupID: m.GroupID, UserID: m.UserID, Role: m.Role, IsManager: m.IsManager,
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
			AppID    string         `json:"appId"`
			Name     string         `json:"name"`
			Metadata map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		g, err := svc.Create(r.Context(), caller, CreateInput{AppID: body.AppID, Name: body.Name, Metadata: body.Metadata})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toGroupResponse(g))
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
		items := make([]groupResponse, 0, len(list))
		for _, g := range list {
			items = append(items, toGroupResponse(g))
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
		g, err := svc.Get(r.Context(), caller, r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toGroupResponse(g))
	}
}

func updateHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Name     *string        `json:"name"`
			Status   *Status        `json:"status"`
			Metadata map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		g, err := svc.Update(r.Context(), caller, r.PathValue("id"),
			UpdateInput{Name: body.Name, Status: body.Status, Metadata: body.Metadata})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toGroupResponse(g))
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
		items := make([]memberResponse, 0, len(list))
		for _, m := range list {
			items = append(items, toMemberResponse(m))
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
			UserID    string `json:"userId"`
			Role      string `json:"role"`
			IsManager bool   `json:"isManager"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.AddMember(r.Context(), caller, r.PathValue("id"),
			AddMemberInput{UserID: body.UserID, Role: body.Role, IsManager: body.IsManager})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toMemberResponse(m))
	}
}

func updateMemberHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, ok := callerID(w, r)
		if !ok {
			return
		}
		var body struct {
			Role      *string `json:"role"`
			IsManager *bool   `json:"isManager"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		m, err := svc.UpdateMember(r.Context(), caller, r.PathValue("id"), r.PathValue("userId"),
			UpdateMemberInput{Role: body.Role, IsManager: body.IsManager})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toMemberResponse(m))
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
	case errors.Is(err, ErrAlreadyMember):
		apperr.Write(w, r, apperr.New(apperr.CodeConflict, err.Error()))
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrManagerRequired):
		apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, err.Error()))
	default:
		slog.Default().Error("unhandled groups error",
			"error", err, "correlationId", correlation.FromContext(r.Context()))
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
