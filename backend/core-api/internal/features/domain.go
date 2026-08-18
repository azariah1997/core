// Package features is the platform's feature-flag service: "applications
// ask isEnabled(feature, context)" is the roadmap's own framing for the
// entry point (Service.Evaluate). Feature/FeatureRule/FeatureEvaluation
// are the three constructs it names.
package features

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Feature is a named, per-app flag. Enabled is a master kill-switch -
// false always evaluates the feature off, before any rule is considered.
type Feature struct {
	ID          string
	AppID       string
	Key         string // product-defined, e.g. "new-checkout-flow"
	Name        string
	Description string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RuleConditions holds every optional targeting dimension the roadmap
// names, besides "app" (a Feature is already scoped to one AppID, so
// targeting by app needs no per-rule condition). Every field is
// independently optional; an empty/nil field means "no constraint on
// this dimension," not "must be empty."
type RuleConditions struct {
	Environments []string `json:"environments,omitempty"`
	UserIDs      []string `json:"userIds,omitempty"`
	TenantIDs    []string `json:"tenantIds,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
	Countries    []string `json:"countries,omitempty"`
	MinVersion   string   `json:"minVersion,omitempty"`
	MaxVersion   string   `json:"maxVersion,omitempty"`
	// Percentage is 0-100, deterministically bucketed per (feature key,
	// EvaluationContext.UserID) so the same user always gets the same
	// in/out result for a given rollout percentage - a real rollout
	// property, not a coin flip on every call.
	Percentage *int `json:"percentage,omitempty"`
}

// FeatureRule is one targeting rule. Rules for a Feature are evaluated in
// ascending Priority order; the first whose Conditions all match wins.
type FeatureRule struct {
	ID         string
	FeatureID  string
	Priority   int
	Conditions RuleConditions
	// Enabled is what the feature evaluates to when this rule matches -
	// usually true, but false lets a narrower, higher-priority rule
	// explicitly exclude a segment before a broader later rule would
	// otherwise include it.
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EvaluationContext is the "context" in isEnabled(feature, context).
type EvaluationContext struct {
	Environment string
	UserID      string
	TenantID    string
	Platform    string
	Country     string
	Version     string
}

// FeatureEvaluation is the roadmap's third construct - the answer to
// isEnabled, with the reasoning behind it. It's a computed value, never
// persisted: a feature-flag check can happen on effectively every
// request in a real system, and logging every evaluation would need
// sampling/aggregation infrastructure this phase doesn't build -
// deliberately out of scope, unlike JobAttempt or NotificationDelivery,
// where each row represents an inherently rarer, individually
// significant event.
type FeatureEvaluation struct {
	FeatureKey    string
	Enabled       bool
	Reason        string
	MatchedRuleID string // empty if no rule matched
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound     = errors.New("feature not found")
	ErrRuleNotFound = errors.New("feature rule not found")
	ErrKeyTaken     = errors.New("a feature with this key already exists for this app")
	ErrForbidden    = errors.New("not permitted to manage features")
)

type CreateFeatureInput struct {
	AppID       string
	Key         string
	Name        string
	Description string
	Enabled     *bool // defaults to true
}

func (in CreateFeatureInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case strings.TrimSpace(in.Key) == "":
		return &ValidationError{"key is required"}
	case strings.TrimSpace(in.Name) == "":
		return &ValidationError{"name is required"}
	}
	return nil
}

type UpdateFeatureInput struct {
	Name        *string
	Description *string
	Enabled     *bool
}

func (in UpdateFeatureInput) Validate() error {
	if in.Name == nil && in.Description == nil && in.Enabled == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return &ValidationError{"name cannot be empty"}
	}
	return nil
}

type CreateRuleInput struct {
	Priority   int
	Conditions RuleConditions
	Enabled    bool
}

func (in CreateRuleInput) Validate() error {
	if in.Conditions.Percentage != nil && (*in.Conditions.Percentage < 0 || *in.Conditions.Percentage > 100) {
		return &ValidationError{"percentage must be between 0 and 100"}
	}
	return nil
}

type UpdateRuleInput struct {
	Priority   *int
	Conditions *RuleConditions
	Enabled    *bool
}

func (in UpdateRuleInput) Validate() error {
	if in.Priority == nil && in.Conditions == nil && in.Enabled == nil {
		return &ValidationError{"at least one field must be provided"}
	}
	if in.Conditions != nil && in.Conditions.Percentage != nil && (*in.Conditions.Percentage < 0 || *in.Conditions.Percentage > 100) {
		return &ValidationError{"percentage must be between 0 and 100"}
	}
	return nil
}

// Repository is the storage-agnostic boundary.
type Repository interface {
	CreateFeature(ctx context.Context, in CreateFeatureInput) (Feature, error)
	GetFeature(ctx context.Context, appID, key string) (Feature, error)
	ListFeatures(ctx context.Context, appID string) ([]Feature, error)
	UpdateFeature(ctx context.Context, appID, key string, in UpdateFeatureInput) (Feature, error)

	CreateRule(ctx context.Context, featureID string, in CreateRuleInput) (FeatureRule, error)
	ListRules(ctx context.Context, featureID string) ([]FeatureRule, error)
	GetRule(ctx context.Context, ruleID string) (FeatureRule, error)
	UpdateRule(ctx context.Context, ruleID string, in UpdateRuleInput) (FeatureRule, error)
	DeleteRule(ctx context.Context, ruleID string) error
}
