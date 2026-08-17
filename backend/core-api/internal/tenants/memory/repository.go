// Package memory is an in-memory tenants.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/tenants"
)

type Repository struct {
	mu          sync.Mutex
	tenants     map[string]tenants.Tenant
	memberships map[string]tenants.Membership // key: tenantID + "|" + userID
	slugs       map[string]string             // key: appID + "|" + slug -> tenantID
}

func New() *Repository {
	return &Repository{
		tenants:     map[string]tenants.Tenant{},
		memberships: map[string]tenants.Membership{},
		slugs:       map[string]string{},
	}
}

func membershipKey(tenantID, userID string) string { return tenantID + "|" + userID }
func slugKey(appID, slug string) string            { return appID + "|" + slug }

func (r *Repository) Create(ctx context.Context, ownerUserID string, in tenants.CreateInput) (tenants.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sk := slugKey(in.AppID, in.Slug)
	if _, taken := r.slugs[sk]; taken {
		return tenants.Tenant{}, tenants.ErrSlugTaken
	}

	now := time.Now().UTC()
	t := tenants.Tenant{
		ID: uuid.NewString(), AppID: in.AppID, Slug: in.Slug, Name: in.Name,
		Status: tenants.StatusActive, CreatedAt: now, UpdatedAt: now,
	}
	r.tenants[t.ID] = t
	r.slugs[sk] = t.ID
	r.memberships[membershipKey(t.ID, ownerUserID)] = tenants.Membership{
		ID: uuid.NewString(), TenantID: t.ID, UserID: ownerUserID, Role: tenants.RoleOwner, CreatedAt: now,
	}
	return t, nil
}

func (r *Repository) Get(ctx context.Context, id string) (tenants.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tenants[id]
	if !ok {
		return tenants.Tenant{}, tenants.ErrNotFound
	}
	return t, nil
}

func (r *Repository) ListForUser(ctx context.Context, userID string) ([]tenants.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []tenants.Tenant
	for _, m := range r.memberships {
		if m.UserID == userID {
			list = append(list, r.tenants[m.TenantID])
		}
	}
	return list, nil
}

func (r *Repository) Update(ctx context.Context, id string, in tenants.UpdateInput) (tenants.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tenants[id]
	if !ok {
		return tenants.Tenant{}, tenants.ErrNotFound
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.Status != nil {
		t.Status = *in.Status
	}
	t.UpdatedAt = time.Now().UTC()
	r.tenants[id] = t
	return t, nil
}

func (r *Repository) GetMembership(ctx context.Context, tenantID, userID string) (tenants.Membership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.memberships[membershipKey(tenantID, userID)]
	if !ok {
		return tenants.Membership{}, tenants.ErrMembershipNotFound
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, tenantID string) ([]tenants.Membership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []tenants.Membership
	for _, m := range r.memberships {
		if m.TenantID == tenantID {
			list = append(list, m)
		}
	}
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, tenantID string, in tenants.AddMemberInput) (tenants.Membership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := membershipKey(tenantID, in.UserID)
	if _, exists := r.memberships[k]; exists {
		return tenants.Membership{}, tenants.ErrAlreadyMember
	}
	m := tenants.Membership{
		ID: uuid.NewString(), TenantID: tenantID, UserID: in.UserID, Role: in.Role, CreatedAt: time.Now().UTC(),
	}
	r.memberships[k] = m
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, tenantID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := membershipKey(tenantID, userID)
	if _, ok := r.memberships[k]; !ok {
		return tenants.ErrMembershipNotFound
	}
	delete(r.memberships, k)
	return nil
}
