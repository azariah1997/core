package api

import (
	"context"

	"github.com/example/core-platform/backend/core-api/internal/authz"
	"github.com/example/core-platform/backend/core-api/internal/users"
)

// profileAccessChecker implements users.AccessChecker using authz.Service.
// The actual policy for "may subject view target's profile" lives here,
// not in users itself, per the platform rule that applications (and
// domain modules) never implement permission logic independently.
type profileAccessChecker struct {
	authzSvc *authz.Service
}

func newProfileAccessChecker(authzSvc *authz.Service) users.AccessChecker {
	return &profileAccessChecker{authzSvc: authzSvc}
}

func (c *profileAccessChecker) CanViewProfile(ctx context.Context, subjectUserID, targetUserID string) (bool, error) {
	if subjectUserID == targetUserID {
		return true, nil
	}
	// Fine-grained per-profile visibility (e.g. via a real relationship)
	// isn't added here speculatively: the OpenFGA model has no
	// user_profile type or viewer relation yet, and Check()-ing an
	// undefined type/relation is a genuine error, not a graceful "false".
	// Phase 8 (Relationships) adds both the model and this check together,
	// once there's real relationship data to check against.
	return c.authzSvc.IsPlatformAdmin(ctx, subjectUserID)
}

// IsPlatformAdmin satisfies the rest of users.AccessChecker (Phase 25's
// addition, gating List) as a direct passthrough - *authz.Service
// already has the exact method, no translation needed.
func (c *profileAccessChecker) IsPlatformAdmin(ctx context.Context, subjectUserID string) (bool, error) {
	return c.authzSvc.IsPlatformAdmin(ctx, subjectUserID)
}
