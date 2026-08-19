// Package memory is an in-memory billing.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/billing"
)

type Repository struct {
	mu           sync.Mutex
	entitlements map[string]billing.Entitlement
	payments     map[string]billing.Payment
	// key: provider|providerRef, used to detect a redelivered webhook
	// the same way the postgres repository's UNIQUE constraint does.
	paymentsByRef map[string]string
}

func New() *Repository {
	return &Repository{
		entitlements:  map[string]billing.Entitlement{},
		payments:      map[string]billing.Payment{},
		paymentsByRef: map[string]string{},
	}
}

func (r *Repository) GrantEntitlement(ctx context.Context, e billing.Entitlement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entitlements[e.ID] = e
	return nil
}

func (r *Repository) RevokeEntitlement(ctx context.Context, id string) (billing.Entitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entitlements[id]
	if !ok {
		return billing.Entitlement{}, billing.ErrNotFound
	}
	now := time.Now().UTC()
	e.RevokedAt = &now
	r.entitlements[id] = e
	return e, nil
}

func (r *Repository) RevokeBySource(ctx context.Context, source string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for id, e := range r.entitlements {
		if e.Source == source && e.RevokedAt == nil {
			e.RevokedAt = &now
			r.entitlements[id] = e
		}
	}
	return nil
}

func (r *Repository) ListEntitlements(ctx context.Context, userID string) ([]billing.Entitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []billing.Entitlement
	for _, e := range r.entitlements {
		if e.UserID == userID {
			list = append(list, e)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].GrantedAt.After(list[j].GrantedAt) })
	return list, nil
}

func (r *Repository) GetEntitlement(ctx context.Context, id string) (billing.Entitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entitlements[id]
	if !ok {
		return billing.Entitlement{}, billing.ErrNotFound
	}
	return e, nil
}

func refKey(provider, providerRef string) string { return provider + "|" + providerRef }

func (r *Repository) RecordPayment(ctx context.Context, p billing.Payment) (billing.Payment, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := refKey(p.Provider, p.ProviderRef)
	if _, exists := r.paymentsByRef[k]; exists {
		return billing.Payment{}, false, nil
	}
	r.payments[p.ID] = p
	r.paymentsByRef[k] = p.ID
	return p, true, nil
}

func (r *Repository) ListPayments(ctx context.Context, userID string) ([]billing.Payment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []billing.Payment
	for _, p := range r.payments {
		if p.UserID == userID {
			list = append(list, p)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}
