package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/example/core-platform/backend/core-api/internal/identity"
	"github.com/example/core-platform/backend/core-api/internal/users"
	"github.com/example/core-platform/packages/go/platformkit/apperr"
)

// identityLinkerFunc adapts identity.Service.LinkUser (a method) into the
// small users.IdentityLinker interface, so users never needs to import
// identity to be usable - only this composition layer does.
type identityLinkerFunc func(ctx context.Context, identityID, userID string) error

func (f identityLinkerFunc) LinkUser(ctx context.Context, identityID, userID string) error {
	return f(ctx, identityID, userID)
}

// requireUser chains identity.Middleware with resolving the caller's own
// platform User, provisioning and linking one on first login. This is
// where Phase 3 (identity) and Phase 4 (users) actually meet.
func requireUser(identitySvc *identity.Service, usersSvc *users.Service) func(http.Handler) http.Handler {
	linker := identityLinkerFunc(identitySvc.LinkUser)
	return func(next http.Handler) http.Handler {
		return identity.Middleware(identitySvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := identity.FromContext(r.Context())
			claims, _ := identity.ClaimsFromContext(r.Context())

			displayName := claims.Username
			if displayName == "" {
				displayName = claims.Email
			}

			user, err := usersSvc.EnsureForIdentity(r.Context(), linker, id.ID, id.UserID, displayName)
			if err != nil {
				var validationErr *users.ValidationError
				if errors.As(err, &validationErr) {
					apperr.Write(w, r, apperr.New(apperr.CodeValidation, validationErr.Message))
					return
				}
				apperr.Write(w, r, apperr.New(apperr.CodeInternal, "failed to resolve user"))
				return
			}

			next.ServeHTTP(w, r.WithContext(users.WithUser(r.Context(), user)))
		}))
	}
}

// RestrictionChecker is satisfied directly by *trustsafety.Service.
type RestrictionChecker interface {
	IsRestricted(ctx context.Context, userID string) (bool, string, error)
}

// requireActive layers Phase 21's Suspension/Ban enforcement on top of
// requireUser, in exactly one place, rather than every module checking
// it independently - the same "one entry point, not per-module logic"
// principle authz.Service.Can already establishes for permissions.
//
// Deliberately NOT applied to every route: users (so a suspended/banned
// caller can still see their own profile and status) and trustsafety
// itself (so they can still file an appeal) are wired with plain
// requireUser in router.go instead - a restricted user must never be
// locked out of knowing why, or contesting it.
func requireActive(identitySvc *identity.Service, usersSvc *users.Service, restrictions RestrictionChecker) func(http.Handler) http.Handler {
	base := requireUser(identitySvc, usersSvc)
	return func(next http.Handler) http.Handler {
		return base(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := users.FromContext(r.Context())
			restricted, reason, err := restrictions.IsRestricted(r.Context(), u.ID)
			if err != nil {
				apperr.Write(w, r, apperr.New(apperr.CodeInternal, "failed to check account restrictions"))
				return
			}
			if restricted {
				apperr.Write(w, r, apperr.New(apperr.CodeAccessDenied, "account is restricted: "+reason))
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}
