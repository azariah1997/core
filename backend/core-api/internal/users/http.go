package users

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/example/core-platform/packages/go/platformkit/apperr"
	"github.com/example/core-platform/packages/go/platformkit/httpx"
)

type contextKey struct{}

// WithUser attaches the caller's own resolved User to ctx. Called by
// whoever composes the "require an authenticated User" middleware chain
// (the api package, which knows about identity) - not by this package.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// FromContext returns the User attached by WithUser, if any.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(contextKey{}).(User)
	return u, ok
}

// RegisterRoutes wires the User endpoints. requireUser must resolve the
// caller's own User (provisioning one on first login) and attach it via
// WithUser before calling next; requireAuth only needs the caller to be
// authenticated, since GET /v1/users/{id} is about a possibly-different
// user. This package defines neither middleware - composing identity with
// users is the api package's job, not a users concern.
func RegisterRoutes(mux *http.ServeMux, svc *Service, requireUser, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /v1/users/me", requireUser(http.HandlerFunc(meHandler)))
	mux.Handle("PATCH /v1/users/me", requireUser(updateMeHandler(svc)))
	mux.Handle("GET /v1/users/{id}", requireAuth(getHandler(svc)))
}

type userResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	AvatarRef   string `json:"avatarRef,omitempty"`
	Locale      string `json:"locale"`
	Timezone    string `json:"timezone"`
	Status      Status `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func toResponse(u User) userResponse {
	return userResponse{
		ID: u.ID, DisplayName: u.DisplayName, AvatarRef: u.AvatarRef,
		Locale: u.Locale, Timezone: u.Timezone, Status: u.Status,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt: u.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	u, ok := FromContext(r.Context())
	if !ok {
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
		return
	}
	httpx.JSON(w, http.StatusOK, toResponse(u))
}

func updateMeHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok {
			apperr.Write(w, r, apperr.New(apperr.CodeInternal, "user missing from context"))
			return
		}
		var body struct {
			DisplayName *string `json:"displayName"`
			AvatarRef   *string `json:"avatarRef"`
			Locale      *string `json:"locale"`
			Timezone    *string `json:"timezone"`
			Status      *Status `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apperr.Write(w, r, apperr.New(apperr.CodeValidation, "invalid JSON body"))
			return
		}
		updated, err := svc.Update(r.Context(), u.ID, UpdateInput{
			DisplayName: body.DisplayName, AvatarRef: body.AvatarRef,
			Locale: body.Locale, Timezone: body.Timezone, Status: body.Status,
		})
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(updated))
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDomainError(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toResponse(u))
	}
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *ValidationError
	switch {
	case errors.As(err, &validationErr):
		apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrDeleted):
		apperr.Write(w, r, apperr.New(apperr.CodeNotFound, "user not found"))
	default:
		apperr.Write(w, r, apperr.New(apperr.CodeInternal, "internal error"))
	}
}
