package aigateway

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Provider is "create a provider-neutral interface" - the roadmap's own
// instruction - satisfied by exactly one real implementation this phase
// ships (aigateway/ollama, genuine local inference, no API key needed)
// and structurally ready for OpenAI/Anthropic/Google adapters later
// with zero changes to this package. Complete takes the actual backing
// model name (resolved from a ModelAlias by Service.Complete before
// calling this) - a Provider never sees or cares about aliases.
type Provider interface {
	Name() string
	Complete(ctx context.Context, model string, messages []Message, maxTokens int, temperature float64) (ProviderResult, error)
}

type ProviderResult struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
}

// AdminChecker mirrors every other module's - satisfied directly by
// *authz.Service.
type AdminChecker interface {
	IsPlatformAdmin(ctx context.Context, userID string) (bool, error)
}

// RateLimiter is satisfied directly by *platformkit/ratelimit.Limiter -
// this phase's "quotas" capability, the same package Phase 21
// introduced and Phase 23 already reused for an unrelated purpose.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// AuditRecorder is a narrow, primitive-typed interface - so this
// package never imports internal/audit directly - satisfied by a small
// adapter over *audit.Service (internal/api/aigateway_adapter.go), the
// same consumer-defined-interface pattern authz.AuditRecorder already
// established for Phase 19's audit integration. Unlike authz's case,
// there's no construction-order cycle here (audit.Service doesn't
// depend on aigateway), so no two-phase wiring is needed - this can be
// constructed after audit.Service already exists.
type AuditRecorder interface {
	RecordCompletion(ctx context.Context, actorUserID, action, resourceID string, metadata map[string]any) error
}

const (
	quotaLimit  = 60
	quotaWindow = time.Minute

	defaultTimeout = 30 * time.Second

	debugListDefaultLimit = 50
	debugListMaxLimit     = 200
)

type Service struct {
	repo      Repository
	admin     AdminChecker
	limiter   RateLimiter
	audit     AuditRecorder
	providers map[string]Provider
	routes    map[string]Route
	timeout   time.Duration
}

func NewService(repo Repository, admin AdminChecker, limiter RateLimiter, audit AuditRecorder) *Service {
	return &Service{
		repo: repo, admin: admin, limiter: limiter, audit: audit,
		providers: map[string]Provider{}, routes: map[string]Route{}, timeout: defaultTimeout,
	}
}

func (s *Service) RegisterProvider(p Provider) { s.providers[p.Name()] = p }
func (s *Service) RegisterRoute(r Route)       { s.routes[r.Alias] = r }

// Complete is the one entry point every product calls - "products must
// not call AI vendors directly" is enforced by this being the only
// path to a Provider at all; nothing outside this package ever holds a
// Provider reference. Quota is checked once per call (this phase's
// deliberately simple "N completions per window per caller" quota, the
// same call-count-limiting shape Phase 21/23 already established with
// this exact RateLimiter type - a real token-quantity quota, summed
// from Completion.TotalTokens, is a natural extension the Repository
// this phase built already has the data for, not implemented as a live
// gate yet).
func (s *Service) Complete(ctx context.Context, callerID string, in CompletionInput) (Completion, error) {
	if err := in.Validate(); err != nil {
		return Completion{}, err
	}
	allowed, err := s.limiter.Allow(ctx, "aigateway:complete:"+callerID, quotaLimit, quotaWindow)
	if err != nil {
		return Completion{}, err
	}
	if !allowed {
		return Completion{}, ErrRateLimited
	}
	route, ok := s.routes[in.ModelAlias]
	if !ok {
		return Completion{}, ErrUnknownRoute
	}

	var lastErr error
	for _, step := range route.Steps {
		provider, ok := s.providers[step.Provider]
		if !ok {
			lastErr = errors.New("provider " + step.Provider + " not registered")
			continue
		}
		result, latency, err := s.attempt(ctx, provider, step.Model, in)
		if err != nil {
			lastErr = err
			continue
		}
		c := Completion{
			ID: uuid.NewString(), UserID: callerID, AppID: in.AppID, ModelAlias: in.ModelAlias,
			Provider: step.Provider, Model: step.Model, PromptKey: in.PromptKey, PromptVersion: in.PromptVersion,
			PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens,
			TotalTokens: result.PromptTokens + result.CompletionTokens,
			CostCents:   cost(route, result.PromptTokens, result.CompletionTokens),
			LatencyMS:   latency.Milliseconds(), FinishReason: result.FinishReason,
			Status: StatusCompleted, CreatedAt: time.Now().UTC(),
		}
		if err := s.repo.RecordCompletion(ctx, c); err != nil {
			return Completion{}, err
		}
		s.recordAudit(ctx, callerID, c)
		c.Text = result.Text
		return c, nil
	}

	// Every step in the fallback chain failed - still recorded, so cost/
	// usage/audit visibility covers failures, not only successes.
	errMsg := "no route steps configured"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	c := Completion{
		ID: uuid.NewString(), UserID: callerID, AppID: in.AppID, ModelAlias: in.ModelAlias,
		PromptKey: in.PromptKey, PromptVersion: in.PromptVersion, Status: StatusFailed, Error: errMsg, CreatedAt: time.Now().UTC(),
	}
	_ = s.repo.RecordCompletion(ctx, c)
	s.recordAudit(ctx, callerID, c)
	return Completion{}, ErrAllStepsFailed
}

// attempt wraps one provider call with this phase's "timeouts"
// capability - context.WithTimeout, not left to whatever default (or
// lack of one) the underlying HTTP client happens to have.
func (s *Service) attempt(ctx context.Context, p Provider, model string, in CompletionInput) (ProviderResult, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	start := time.Now()
	result, err := p.Complete(ctx, model, in.Messages, in.MaxTokens, in.Temperature)
	return result, time.Since(start), err
}

// cost is cents-per-million-tokens times tokens-in-millions - Ollama's
// registered Route prices both at 0, so this honestly returns 0 for
// this phase's one real provider while remaining the real mechanism a
// paid provider's Route would use.
func cost(route Route, promptTokens, completionTokens int) float64 {
	return float64(promptTokens)/1_000_000*route.PricePerMillionPromptTokens +
		float64(completionTokens)/1_000_000*route.PricePerMillionCompletionTokens
}

// recordAudit is best-effort - a failure to record is not surfaced to
// the caller, so an audit-service hiccup can never block a completion
// that otherwise succeeded, the same "best-effort side effect can't
// fail the primary operation" principle authz.Service.AssignRole
// already applies to its own audit integration.
func (s *Service) recordAudit(ctx context.Context, callerID string, c Completion) {
	action := "ai.completion.succeeded"
	if c.Status == StatusFailed {
		action = "ai.completion.failed"
	}
	_ = s.audit.RecordCompletion(ctx, callerID, action, c.ID, map[string]any{
		"modelAlias": c.ModelAlias, "provider": c.Provider, "model": c.Model,
		"promptKey": c.PromptKey, "promptVersion": c.PromptVersion, "totalTokens": c.TotalTokens, "costCents": c.CostCents,
	})
}

func (s *Service) ListCompletions(ctx context.Context, callerID, targetUserID string, limit int) ([]Completion, error) {
	if err := s.requireOwnerOrAdmin(ctx, callerID, targetUserID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > debugListMaxLimit {
		limit = debugListDefaultLimit
	}
	return s.repo.ListCompletions(ctx, targetUserID, limit)
}

// Routes returns the currently registered model aliases and their
// fallback chains - what GET /v1/ai/models exposes so a caller can
// discover which aliases exist without needing to know any vendor's
// model naming.
func (s *Service) Routes() []Route {
	list := make([]Route, 0, len(s.routes))
	for _, r := range s.routes {
		list = append(list, r)
	}
	return list
}

func (s *Service) requireOwnerOrAdmin(ctx context.Context, callerID, ownerID string) error {
	if callerID == ownerID {
		return nil
	}
	isAdmin, err := s.admin.IsPlatformAdmin(ctx, callerID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrForbidden
	}
	return nil
}
