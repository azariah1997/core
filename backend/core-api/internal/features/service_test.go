package features_test

import (
	"context"
	"errors"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/features"
	"github.com/example/core-platform/backend/core-api/internal/features/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

func newService(admins map[string]bool) *features.Service {
	return features.NewService(memory.New(), fakeAdmin{admins: admins})
}

func TestCreateFeatureRequiresAdmin(t *testing.T) {
	svc := newService(nil)
	_, err := svc.CreateFeature(context.Background(), "u1", features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"})
	if !errors.Is(err, features.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestCreateFeatureRejectsDuplicateKey(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	in := features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"}
	if _, err := svc.CreateFeature(ctx, "admin", in); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateFeature(ctx, "admin", in); !errors.Is(err, features.ErrKeyTaken) {
		t.Fatalf("expected ErrKeyTaken, got %v", err)
	}
}

func TestGetAndListAreOpenToAnyAuthenticatedCaller(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.GetFeature(ctx, "app-1", "x"); err != nil {
		t.Fatalf("expected a non-admin read to succeed, got %v", err)
	}
	list, err := svc.ListFeatures(ctx, "app-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 listed feature, got %v err=%v", list, err)
	}
}

func TestUpdateFeatureRequiresAdmin(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	newName := "Y"
	_, err := svc.UpdateFeature(ctx, "u1", "app-1", "x", features.UpdateFeatureInput{Name: &newName})
	if !errors.Is(err, features.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestKillSwitchViaUpdateFeature(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateRule(ctx, "admin", "app-1", "x", features.CreateRuleInput{Enabled: true}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	eval, err := svc.Evaluate(ctx, "app-1", "x", features.EvaluationContext{})
	if err != nil || !eval.Enabled {
		t.Fatalf("expected enabled before kill switch, got %+v err=%v", eval, err)
	}

	off := false
	if _, err := svc.UpdateFeature(ctx, "admin", "app-1", "x", features.UpdateFeatureInput{Enabled: &off}); err != nil {
		t.Fatalf("update: %v", err)
	}
	eval, err = svc.Evaluate(ctx, "app-1", "x", features.EvaluationContext{})
	if err != nil || eval.Enabled {
		t.Fatalf("expected the kill switch to override the matching rule, got %+v err=%v", eval, err)
	}
}

func TestCreateRuleRequiresAdminAndExistingFeature(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()

	_, err := svc.CreateRule(ctx, "u1", "app-1", "x", features.CreateRuleInput{})
	if !errors.Is(err, features.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin, got %v", err)
	}

	_, err = svc.CreateRule(ctx, "admin", "app-1", "does-not-exist", features.CreateRuleInput{})
	if !errors.Is(err, features.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a missing feature, got %v", err)
	}
}

func TestRuleMustBelongToTheNamedFeature(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "feature-a", Name: "A"}); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "feature-b", Name: "B"}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	rule, err := svc.CreateRule(ctx, "admin", "app-1", "feature-a", features.CreateRuleInput{})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	// Try to delete feature-a's rule through feature-b's URL path.
	err = svc.DeleteRule(ctx, "admin", "app-1", "feature-b", rule.ID)
	if !errors.Is(err, features.ErrRuleNotFound) {
		t.Fatalf("expected ErrRuleNotFound when a rule doesn't belong to the named feature, got %v", err)
	}
}

func TestEvaluateEndToEndThroughService(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	ctx := context.Background()
	if _, err := svc.CreateFeature(ctx, "admin", features.CreateFeatureInput{AppID: "app-1", Key: "x", Name: "X"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateRule(ctx, "admin", "app-1", "x", features.CreateRuleInput{
		Enabled: true, Conditions: features.RuleConditions{Environments: []string{"production"}},
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	eval, err := svc.Evaluate(ctx, "app-1", "x", features.EvaluationContext{Environment: "production"})
	if err != nil || !eval.Enabled {
		t.Fatalf("expected production to be enabled, got %+v err=%v", eval, err)
	}
	eval, err = svc.Evaluate(ctx, "app-1", "x", features.EvaluationContext{Environment: "staging"})
	if err != nil || eval.Enabled {
		t.Fatalf("expected staging to be disabled, got %+v err=%v", eval, err)
	}
}
