// Package memory is an in-memory features.Repository for tests.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/features"
)

type Repository struct {
	mu       sync.Mutex
	features map[string]features.Feature     // id
	byAppKey map[string]string               // appID|key -> id
	rules    map[string]features.FeatureRule // id
}

func New() *Repository {
	return &Repository{
		features: map[string]features.Feature{},
		byAppKey: map[string]string{},
		rules:    map[string]features.FeatureRule{},
	}
}

func appKey(appID, key string) string { return appID + "|" + key }

func (r *Repository) CreateFeature(ctx context.Context, in features.CreateFeatureInput) (features.Feature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := appKey(in.AppID, in.Key)
	if _, exists := r.byAppKey[k]; exists {
		return features.Feature{}, features.ErrKeyTaken
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	now := time.Now().UTC()
	f := features.Feature{
		ID: uuid.NewString(), AppID: in.AppID, Key: in.Key, Name: in.Name, Description: in.Description,
		Enabled: enabled, CreatedAt: now, UpdatedAt: now,
	}
	r.features[f.ID] = f
	r.byAppKey[k] = f.ID
	return f, nil
}

func (r *Repository) GetFeature(ctx context.Context, appID, key string) (features.Feature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byAppKey[appKey(appID, key)]
	if !ok {
		return features.Feature{}, features.ErrNotFound
	}
	return r.features[id], nil
}

func (r *Repository) ListFeatures(ctx context.Context, appID string) ([]features.Feature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []features.Feature
	for _, f := range r.features {
		if f.AppID == appID {
			list = append(list, f)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	return list, nil
}

func (r *Repository) UpdateFeature(ctx context.Context, appID, key string, in features.UpdateFeatureInput) (features.Feature, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byAppKey[appKey(appID, key)]
	if !ok {
		return features.Feature{}, features.ErrNotFound
	}
	f := r.features[id]
	if in.Name != nil {
		f.Name = *in.Name
	}
	if in.Description != nil {
		f.Description = *in.Description
	}
	if in.Enabled != nil {
		f.Enabled = *in.Enabled
	}
	f.UpdatedAt = time.Now().UTC()
	r.features[id] = f
	return f, nil
}

func (r *Repository) CreateRule(ctx context.Context, featureID string, in features.CreateRuleInput) (features.FeatureRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	rule := features.FeatureRule{
		ID: uuid.NewString(), FeatureID: featureID, Priority: in.Priority, Conditions: in.Conditions,
		Enabled: in.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	r.rules[rule.ID] = rule
	return rule, nil
}

func (r *Repository) ListRules(ctx context.Context, featureID string) ([]features.FeatureRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []features.FeatureRule
	for _, rule := range r.rules {
		if rule.FeatureID == featureID {
			list = append(list, rule)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Priority < list[j].Priority })
	return list, nil
}

func (r *Repository) GetRule(ctx context.Context, ruleID string) (features.FeatureRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[ruleID]
	if !ok {
		return features.FeatureRule{}, features.ErrRuleNotFound
	}
	return rule, nil
}

func (r *Repository) UpdateRule(ctx context.Context, ruleID string, in features.UpdateRuleInput) (features.FeatureRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[ruleID]
	if !ok {
		return features.FeatureRule{}, features.ErrRuleNotFound
	}
	if in.Priority != nil {
		rule.Priority = *in.Priority
	}
	if in.Conditions != nil {
		rule.Conditions = *in.Conditions
	}
	if in.Enabled != nil {
		rule.Enabled = *in.Enabled
	}
	rule.UpdatedAt = time.Now().UTC()
	r.rules[ruleID] = rule
	return rule, nil
}

func (r *Repository) DeleteRule(ctx context.Context, ruleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rules[ruleID]; !ok {
		return features.ErrRuleNotFound
	}
	delete(r.rules, ruleID)
	return nil
}
