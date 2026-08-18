// Package memory is an in-memory groups.Repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/groups"
)

type Repository struct {
	mu      sync.Mutex
	groups  map[string]groups.Group
	members map[string]groups.GroupMember // key: groupID + "|" + userID
}

func New() *Repository {
	return &Repository{groups: map[string]groups.Group{}, members: map[string]groups.GroupMember{}}
}

func key(groupID, userID string) string { return groupID + "|" + userID }

func (r *Repository) Create(ctx context.Context, creatorUserID string, in groups.CreateInput) (groups.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	g := groups.Group{
		ID: uuid.NewString(), AppID: in.AppID, Name: in.Name, Status: groups.StatusActive,
		Metadata: in.Metadata, CreatedAt: now, UpdatedAt: now,
	}
	r.groups[g.ID] = g
	r.members[key(g.ID, creatorUserID)] = groups.GroupMember{
		ID: uuid.NewString(), GroupID: g.ID, UserID: creatorUserID, Role: "owner", IsManager: true, CreatedAt: now,
	}
	return g, nil
}

func (r *Repository) Get(ctx context.Context, id string) (groups.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.groups[id]
	if !ok {
		return groups.Group{}, groups.ErrNotFound
	}
	return g, nil
}

func (r *Repository) ListForUser(ctx context.Context, userID string) ([]groups.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []groups.Group
	for _, m := range r.members {
		if m.UserID == userID {
			list = append(list, r.groups[m.GroupID])
		}
	}
	return list, nil
}

func (r *Repository) Update(ctx context.Context, id string, in groups.UpdateInput) (groups.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.groups[id]
	if !ok {
		return groups.Group{}, groups.ErrNotFound
	}
	if in.Name != nil {
		g.Name = *in.Name
	}
	if in.Status != nil {
		g.Status = *in.Status
	}
	if in.Metadata != nil {
		g.Metadata = in.Metadata
	}
	g.UpdatedAt = time.Now().UTC()
	r.groups[id] = g
	return g, nil
}

func (r *Repository) GetMembership(ctx context.Context, groupID, userID string) (groups.GroupMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[key(groupID, userID)]
	if !ok {
		return groups.GroupMember{}, groups.ErrMembershipNotFound
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, groupID string) ([]groups.GroupMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []groups.GroupMember
	for _, m := range r.members {
		if m.GroupID == groupID {
			list = append(list, m)
		}
	}
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, groupID string, in groups.AddMemberInput) (groups.GroupMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(groupID, in.UserID)
	if _, exists := r.members[k]; exists {
		return groups.GroupMember{}, groups.ErrAlreadyMember
	}
	role := in.Role
	if role == "" {
		role = "member"
	}
	m := groups.GroupMember{
		ID: uuid.NewString(), GroupID: groupID, UserID: in.UserID, Role: role,
		IsManager: in.IsManager, CreatedAt: time.Now().UTC(),
	}
	r.members[k] = m
	return m, nil
}

func (r *Repository) UpdateMember(ctx context.Context, groupID, userID string, in groups.UpdateMemberInput) (groups.GroupMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(groupID, userID)
	m, ok := r.members[k]
	if !ok {
		return groups.GroupMember{}, groups.ErrMembershipNotFound
	}
	if in.Role != nil {
		m.Role = *in.Role
	}
	if in.IsManager != nil {
		m.IsManager = *in.IsManager
	}
	r.members[k] = m
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, groupID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(groupID, userID)
	if _, ok := r.members[k]; !ok {
		return groups.ErrMembershipNotFound
	}
	delete(r.members, k)
	return nil
}
