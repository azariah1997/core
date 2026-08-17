// Package memory provides in-memory authz.RoleRepository and authz.Provider
// implementations for tests.
package memory

import (
	"context"
	"sync"

	"github.com/example/core-platform/backend/core-api/internal/authz"
)

type RoleRepository struct {
	mu    sync.Mutex
	roles map[string]map[authz.Role]bool
}

func NewRoleRepository() *RoleRepository {
	return &RoleRepository{roles: map[string]map[authz.Role]bool{}}
}

func (r *RoleRepository) RolesFor(ctx context.Context, userID string) ([]authz.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var roles []authz.Role
	for role, ok := range r.roles[userID] {
		if ok {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

func (r *RoleRepository) AssignRole(ctx context.Context, userID string, role authz.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.roles[userID] == nil {
		r.roles[userID] = map[authz.Role]bool{}
	}
	r.roles[userID][role] = true
	return nil
}

func (r *RoleRepository) RevokeRole(ctx context.Context, userID string, role authz.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.roles[userID], role)
	return nil
}

type key struct {
	subject, action, resourceType, resourceID string
}

// Provider is an in-memory authz.Provider for tests - a plain set of
// granted tuples, no relationship-graph traversal.
type Provider struct {
	mu     sync.Mutex
	grants map[key]bool
}

func NewProvider() *Provider {
	return &Provider{grants: map[key]bool{}}
}

func toKey(subjectUserID string, action authz.Action, resource authz.Resource) key {
	return key{subjectUserID, string(action), resource.Type, resource.ID}
}

func (p *Provider) Can(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grants[toKey(subjectUserID, action, resource)], nil
}

func (p *Provider) Grant(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.grants[toKey(subjectUserID, action, resource)] = true
	return nil
}

func (p *Provider) Revoke(ctx context.Context, subjectUserID string, action authz.Action, resource authz.Resource) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.grants, toKey(subjectUserID, action, resource))
	return nil
}
