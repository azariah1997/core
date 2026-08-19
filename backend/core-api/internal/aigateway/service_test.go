package aigateway_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/aigateway"
	"github.com/example/core-platform/backend/core-api/internal/aigateway/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

type fakeLimiter struct{ allow bool }

func (f fakeLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return f.allow, nil
}

type fakeAudit struct{ calls []string }

func (f *fakeAudit) RecordCompletion(ctx context.Context, actorUserID, action, resourceID string, metadata map[string]any) error {
	f.calls = append(f.calls, action)
	return nil
}

type fakeProvider struct {
	name   string
	result aigateway.ProviderResult
	err    error
}

func (p fakeProvider) Name() string { return p.name }
func (p fakeProvider) Complete(ctx context.Context, model string, messages []aigateway.Message, maxTokens int, temperature float64) (aigateway.ProviderResult, error) {
	return p.result, p.err
}

func newService(admins map[string]bool, allow bool) (*aigateway.Service, *fakeAudit) {
	audit := &fakeAudit{}
	return aigateway.NewService(memory.New(), fakeAdmin{admins: admins}, fakeLimiter{allow: allow}, audit), audit
}

func TestCompleteRequiresModelAlias(t *testing.T) {
	svc, _ := newService(nil, true)
	_, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{Messages: []aigateway.Message{{Role: "user", Content: "hi"}}})
	var verr *aigateway.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCompleteRequiresMessages(t *testing.T) {
	svc, _ := newService(nil, true)
	_, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{ModelAlias: "default"})
	var verr *aigateway.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCompleteRejectsUnknownRoute(t *testing.T) {
	svc, _ := newService(nil, true)
	_, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{
		ModelAlias: "nonexistent", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, aigateway.ErrUnknownRoute) {
		t.Fatalf("expected ErrUnknownRoute, got %v", err)
	}
}

func TestCompleteIsRateLimited(t *testing.T) {
	svc, _ := newService(nil, false)
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})
	_, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{
		ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, aigateway.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestCompleteSucceedsAndRecordsUsageAndAudit(t *testing.T) {
	svc, audit := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "ollama", result: aigateway.ProviderResult{Text: "hello", PromptTokens: 10, CompletionTokens: 5, FinishReason: "stop"}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})

	ctx := context.Background()
	c, err := svc.Complete(ctx, "u1", aigateway.CompletionInput{
		ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}}, PromptKey: "greeting", PromptVersion: "v1",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if c.Text != "hello" {
		t.Fatalf("expected the response text to be returned, got %q", c.Text)
	}
	if c.TotalTokens != 15 || c.Status != aigateway.StatusCompleted {
		t.Fatalf("unexpected completion: %+v", c)
	}

	list, err := svc.ListCompletions(ctx, "admin", "u1", 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 recorded completion, got %v err=%v", list, err)
	}
	if list[0].Text != "" {
		t.Fatalf("expected Text to never be persisted, got %q", list[0].Text)
	}
	if len(audit.calls) != 1 || audit.calls[0] != "ai.completion.succeeded" {
		t.Fatalf("expected exactly one succeeded audit call, got %v", audit.calls)
	}
}

func TestCompleteFallsBackToTheNextStepOnFailure(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "primary", err: errors.New("primary is down")})
	svc.RegisterProvider(fakeProvider{name: "backup", result: aigateway.ProviderResult{Text: "backup answered", PromptTokens: 1, CompletionTokens: 1}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{
		{Provider: "primary", Model: "m1"}, {Provider: "backup", Model: "m2"},
	}})

	c, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{
		ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("expected fallback to the backup provider to succeed, got %v", err)
	}
	if c.Provider != "backup" || c.Text != "backup answered" {
		t.Fatalf("expected the backup provider's result, got %+v", c)
	}
}

func TestCompleteReturnsErrorWhenEveryStepFails(t *testing.T) {
	svc, audit := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "primary", err: errors.New("primary is down")})
	svc.RegisterProvider(fakeProvider{name: "backup", err: errors.New("backup is down too")})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{
		{Provider: "primary", Model: "m1"}, {Provider: "backup", Model: "m2"},
	}})

	ctx := context.Background()
	_, err := svc.Complete(ctx, "u1", aigateway.CompletionInput{
		ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if !errors.Is(err, aigateway.ErrAllStepsFailed) {
		t.Fatalf("expected ErrAllStepsFailed, got %v", err)
	}

	list, err := svc.ListCompletions(ctx, "admin", "u1", 10)
	if err != nil || len(list) != 1 || list[0].Status != aigateway.StatusFailed {
		t.Fatalf("expected a failed completion to still be recorded, got %v err=%v", list, err)
	}
	if len(audit.calls) != 1 || audit.calls[0] != "ai.completion.failed" {
		t.Fatalf("expected a failed audit call, got %v", audit.calls)
	}
}

func TestCostIsComputedFromRoutePricing(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "paid", result: aigateway.ProviderResult{Text: "x", PromptTokens: 1_000_000, CompletionTokens: 500_000}})
	svc.RegisterRoute(aigateway.Route{
		Alias: "paid-model", Steps: []aigateway.RouteStep{{Provider: "paid", Model: "m1"}},
		PricePerMillionPromptTokens: 100, PricePerMillionCompletionTokens: 200,
	})

	c, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{
		ModelAlias: "paid-model", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	// 1,000,000 prompt tokens * 100 cents/million + 500,000 completion tokens * 200 cents/million = 100 + 100 = 200 cents.
	if c.CostCents != 200 {
		t.Fatalf("expected cost 200 cents, got %v", c.CostCents)
	}
}

func TestOllamaRouteIsHonestlyFree(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "ollama", result: aigateway.ProviderResult{Text: "x", PromptTokens: 1000, CompletionTokens: 1000}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})

	c, err := svc.Complete(context.Background(), "u1", aigateway.CompletionInput{
		ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if c.CostCents != 0 {
		t.Fatalf("expected local inference to cost 0, got %v", c.CostCents)
	}
}

func TestListCompletionsRequiresOwnerOrAdmin(t *testing.T) {
	svc, _ := newService(map[string]bool{"admin": true}, true)
	svc.RegisterProvider(fakeProvider{name: "ollama", result: aigateway.ProviderResult{Text: "x"}})
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "m1"}}})
	ctx := context.Background()
	if _, err := svc.Complete(ctx, "u1", aigateway.CompletionInput{ModelAlias: "default", Messages: []aigateway.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := svc.ListCompletions(ctx, "stranger", "u1", 10); !errors.Is(err, aigateway.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a stranger, got %v", err)
	}
	if _, err := svc.ListCompletions(ctx, "u1", "u1", 10); err != nil {
		t.Fatalf("expected the owner to list their own usage, got %v", err)
	}
}

func TestRoutesExposesRegisteredAliases(t *testing.T) {
	svc, _ := newService(nil, true)
	svc.RegisterRoute(aigateway.Route{Alias: "default", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})
	svc.RegisterRoute(aigateway.Route{Alias: "fast", Steps: []aigateway.RouteStep{{Provider: "ollama", Model: "qwen2.5:0.5b"}}})
	routes := svc.Routes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 registered routes, got %d", len(routes))
	}
}
