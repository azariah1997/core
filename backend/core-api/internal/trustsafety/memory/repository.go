// Package memory is an in-memory trustsafety.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
)

type Repository struct {
	mu          sync.Mutex
	mutes       map[string]trustsafety.Mute // key: muter|muted
	cases       map[string]trustsafety.ModerationCase
	reports     map[string]trustsafety.Report
	suspensions map[string]trustsafety.Suspension
	bans        map[string]trustsafety.Ban
	appeals     map[string]trustsafety.Appeal
	signals     []trustsafety.AbuseSignal
}

func New() *Repository {
	return &Repository{
		mutes:       map[string]trustsafety.Mute{},
		cases:       map[string]trustsafety.ModerationCase{},
		reports:     map[string]trustsafety.Report{},
		suspensions: map[string]trustsafety.Suspension{},
		bans:        map[string]trustsafety.Ban{},
		appeals:     map[string]trustsafety.Appeal{},
	}
}

func muteKey(muter, muted string) string { return muter + "|" + muted }

func (r *Repository) Mute(ctx context.Context, m trustsafety.Mute) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := muteKey(m.MuterUserID, m.MutedUserID)
	if _, exists := r.mutes[k]; exists {
		return nil
	}
	r.mutes[k] = m
	return nil
}

func (r *Repository) Unmute(ctx context.Context, muterUserID, mutedUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.mutes, muteKey(muterUserID, mutedUserID))
	return nil
}

func (r *Repository) ListMutes(ctx context.Context, muterUserID string) ([]trustsafety.Mute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.Mute
	for _, m := range r.mutes {
		if m.MuterUserID == muterUserID {
			list = append(list, m)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func resourceKey(resourceType, resourceID string) string { return resourceType + "|" + resourceID }

func (r *Repository) OpenOrAttachCase(ctx context.Context, resourceType, resourceID string) (trustsafety.ModerationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.cases {
		if resourceKey(c.ResourceType, c.ResourceID) == resourceKey(resourceType, resourceID) &&
			(c.Status == trustsafety.CaseStatusOpen || c.Status == trustsafety.CaseStatusInReview) {
			c.ReportCount++
			r.cases[id] = c
			return c, nil
		}
	}
	c := trustsafety.ModerationCase{
		ID: uuid.NewString(), ResourceType: resourceType, ResourceID: resourceID,
		Status: trustsafety.CaseStatusOpen, ReportCount: 1, CreatedAt: time.Now().UTC(),
	}
	r.cases[c.ID] = c
	return c, nil
}

func (r *Repository) GetCase(ctx context.Context, id string) (trustsafety.ModerationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok {
		return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
	}
	return c, nil
}

func (r *Repository) ListCases(ctx context.Context, status trustsafety.CaseStatus) ([]trustsafety.ModerationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.ModerationCase
	for _, c := range r.cases {
		if status == "" || c.Status == status {
			list = append(list, c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) ResolveCase(ctx context.Context, id string, in trustsafety.ResolveCaseInput) (trustsafety.ModerationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok {
		return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
	}
	now := time.Now().UTC()
	c.Status, c.Resolution, c.ResolvedAt = in.Status, in.Resolution, &now
	r.cases[id] = c
	return c, nil
}

func (r *Repository) AssignCase(ctx context.Context, id, assigneeUserID string) (trustsafety.ModerationCase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cases[id]
	if !ok {
		return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
	}
	c.AssignedTo, c.Status = assigneeUserID, trustsafety.CaseStatusInReview
	r.cases[id] = c
	return c, nil
}

func (r *Repository) CreateReport(ctx context.Context, rep trustsafety.Report) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports[rep.ID] = rep
	return nil
}

func (r *Repository) GetReport(ctx context.Context, id string) (trustsafety.Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rep, ok := r.reports[id]
	if !ok {
		return trustsafety.Report{}, trustsafety.ErrNotFound
	}
	return rep, nil
}

func (r *Repository) ListReports(ctx context.Context, caseID string) ([]trustsafety.Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.Report
	for _, rep := range r.reports {
		if rep.CaseID == caseID {
			list = append(list, rep)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) CreateSuspension(ctx context.Context, s trustsafety.Suspension) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suspensions[s.ID] = s
	return nil
}

func (r *Repository) GetSuspension(ctx context.Context, id string) (trustsafety.Suspension, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.suspensions[id]
	if !ok {
		return trustsafety.Suspension{}, trustsafety.ErrNotFound
	}
	return s, nil
}

func (r *Repository) ListSuspensions(ctx context.Context, userID string) ([]trustsafety.Suspension, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.Suspension
	for _, s := range r.suspensions {
		if s.UserID == userID {
			list = append(list, s)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) LiftSuspension(ctx context.Context, id, liftedBy string) (trustsafety.Suspension, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.suspensions[id]
	if !ok {
		return trustsafety.Suspension{}, trustsafety.ErrNotFound
	}
	now := time.Now().UTC()
	s.LiftedAt, s.LiftedBy = &now, liftedBy
	r.suspensions[id] = s
	return s, nil
}

func (r *Repository) CreateBan(ctx context.Context, b trustsafety.Ban) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bans[b.ID] = b
	return nil
}

func (r *Repository) GetBan(ctx context.Context, id string) (trustsafety.Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bans[id]
	if !ok {
		return trustsafety.Ban{}, trustsafety.ErrNotFound
	}
	return b, nil
}

func (r *Repository) ListBans(ctx context.Context, userID string) ([]trustsafety.Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.Ban
	for _, b := range r.bans {
		if b.UserID == userID {
			list = append(list, b)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) LiftBan(ctx context.Context, id, liftedBy string) (trustsafety.Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bans[id]
	if !ok {
		return trustsafety.Ban{}, trustsafety.ErrNotFound
	}
	now := time.Now().UTC()
	b.LiftedAt, b.LiftedBy = &now, liftedBy
	r.bans[id] = b
	return b, nil
}

func (r *Repository) CreateAppeal(ctx context.Context, a trustsafety.Appeal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.appeals[a.ID] = a
	return nil
}

func (r *Repository) GetAppeal(ctx context.Context, id string) (trustsafety.Appeal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.appeals[id]
	if !ok {
		return trustsafety.Appeal{}, trustsafety.ErrNotFound
	}
	return a, nil
}

func (r *Repository) ListAppeals(ctx context.Context, userID string) ([]trustsafety.Appeal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.Appeal
	for _, a := range r.appeals {
		if a.UserID == userID {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (r *Repository) ReviewAppeal(ctx context.Context, id string, status trustsafety.AppealStatus, reviewedBy string) (trustsafety.Appeal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.appeals[id]
	if !ok {
		return trustsafety.Appeal{}, trustsafety.ErrNotFound
	}
	now := time.Now().UTC()
	a.Status, a.ReviewedBy, a.ReviewedAt = status, reviewedBy, &now
	r.appeals[id] = a
	return a, nil
}

func (r *Repository) RecordSignal(ctx context.Context, sig trustsafety.AbuseSignal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, sig)
	return nil
}

func (r *Repository) ListSignals(ctx context.Context, resourceType, resourceID string) ([]trustsafety.AbuseSignal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []trustsafety.AbuseSignal
	for _, s := range r.signals {
		if s.ResourceType == resourceType && s.ResourceID == resourceID {
			list = append(list, s)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].RecordedAt.After(list[j].RecordedAt) })
	return list, nil
}
