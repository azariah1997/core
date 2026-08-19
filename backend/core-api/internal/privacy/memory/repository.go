// Package memory is an in-memory privacy.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/privacy"
)

type Repository struct {
	mu        sync.Mutex
	consents  []privacy.Consent
	prefs     map[string]map[string]bool
	policies  map[string]privacy.RetentionPolicy // key: appID+"|"+resourceType
	exports   map[string]privacy.ExportRequest
	deletions map[string]privacy.DeletionRequest
}

func New() *Repository {
	return &Repository{
		prefs:     map[string]map[string]bool{},
		policies:  map[string]privacy.RetentionPolicy{},
		exports:   map[string]privacy.ExportRequest{},
		deletions: map[string]privacy.DeletionRequest{},
	}
}

func (r *Repository) RecordConsent(ctx context.Context, c privacy.Consent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.consents = append(r.consents, c)
	return nil
}

func (r *Repository) ListConsent(ctx context.Context, userID string) ([]privacy.Consent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []privacy.Consent
	for _, c := range r.consents {
		if c.UserID == userID {
			list = append(list, c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].RecordedAt.After(list[j].RecordedAt) })
	return list, nil
}

func (r *Repository) GetPreferences(ctx context.Context, userID string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for k, v := range r.prefs[userID] {
		out[k] = v
	}
	return out, nil
}

func (r *Repository) SetPreferences(ctx context.Context, userID string, prefs map[string]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prefs[userID] == nil {
		r.prefs[userID] = map[string]bool{}
	}
	for k, v := range prefs {
		r.prefs[userID][k] = v
	}
	return nil
}

func policyKey(appID, resourceType string) string { return appID + "|" + resourceType }

func (r *Repository) UpsertRetentionPolicy(ctx context.Context, p privacy.RetentionPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := policyKey(p.AppID, p.ResourceType)
	if existing, ok := r.policies[k]; ok {
		p.ID = existing.ID
		p.CreatedAt = existing.CreatedAt
		p.CreatedBy = existing.CreatedBy
	}
	r.policies[k] = p
	return nil
}

func (r *Repository) ListRetentionPolicies(ctx context.Context) ([]privacy.RetentionPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []privacy.RetentionPolicy
	for _, p := range r.policies {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ResourceType < list[j].ResourceType })
	return list, nil
}

func (r *Repository) CreateExportRequest(ctx context.Context, req privacy.ExportRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exports[req.ID] = req
	return nil
}

func (r *Repository) SetExportWorkflowRef(ctx context.Context, id, workflowID, runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.exports[id]
	if !ok {
		return privacy.ErrNotFound
	}
	req.WorkflowID, req.RunID, req.Status = workflowID, runID, privacy.StatusRunning
	r.exports[id] = req
	return nil
}

func (r *Repository) CompleteExportRequest(ctx context.Context, id string, status privacy.RequestStatus, objectKey, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.exports[id]
	if !ok {
		return privacy.ErrNotFound
	}
	now := time.Now().UTC()
	req.Status, req.ObjectKey, req.Error, req.CompletedAt = status, objectKey, errMsg, &now
	r.exports[id] = req
	return nil
}

func (r *Repository) GetExportRequest(ctx context.Context, id string) (privacy.ExportRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.exports[id]
	if !ok {
		return privacy.ExportRequest{}, privacy.ErrNotFound
	}
	return req, nil
}

func (r *Repository) CreateDeletionRequest(ctx context.Context, req privacy.DeletionRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deletions[req.ID] = req
	return nil
}

func (r *Repository) SetDeletionWorkflowRef(ctx context.Context, id, workflowID, runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.deletions[id]
	if !ok {
		return privacy.ErrNotFound
	}
	req.WorkflowID, req.RunID, req.Status = workflowID, runID, privacy.StatusRunning
	r.deletions[id] = req
	return nil
}

func (r *Repository) CompleteDeletionRequest(ctx context.Context, id string, status privacy.RequestStatus, results map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.deletions[id]
	if !ok {
		return privacy.ErrNotFound
	}
	now := time.Now().UTC()
	req.Status, req.Results, req.CompletedAt = status, results, &now
	r.deletions[id] = req
	return nil
}

func (r *Repository) GetDeletionRequest(ctx context.Context, id string) (privacy.DeletionRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req, ok := r.deletions[id]
	if !ok {
		return privacy.DeletionRequest{}, privacy.ErrNotFound
	}
	return req, nil
}
