// Package aigateway implements Phase 24: "products must not call AI
// vendors directly." A provider-neutral interface (Provider, below)
// supports OpenAI/Anthropic/Google/local-models/Ollama/vLLM, with
// model routing, quotas, token usage, cost tracking, audit,
// prompt/version metadata, timeouts, and fallback - the roadmap's own
// named capability list, each satisfied concretely (see service.go).
package aigateway

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Message is a provider-neutral chat turn - the same {role, content}
// shape every major provider's API already converged on, so this
// package doesn't invent a fourth incompatible one.
type Message struct {
	Role    string
	Content string
}

// CompletionInput is what a caller submits. ModelAlias is a
// product-facing name ("default", "fast", "reasoning") resolved to an
// actual provider+model via a registered Route - callers never name a
// vendor model directly, which is the literal mechanism enforcing
// "products must not call AI vendors directly." PromptKey/PromptVersion
// are the roadmap's own named "prompt/version metadata" capability -
// free-form, product-defined, the same convention as every other
// Type/Key-shaped field in this repo.
type CompletionInput struct {
	ModelAlias    string
	Messages      []Message
	MaxTokens     int
	Temperature   float64
	AppID         string
	PromptKey     string
	PromptVersion string
}

func (in CompletionInput) Validate() error {
	if strings.TrimSpace(in.ModelAlias) == "" {
		return &ValidationError{"modelAlias is required"}
	}
	if len(in.Messages) == 0 {
		return &ValidationError{"messages must not be empty"}
	}
	return nil
}

// Completion is the persisted record of one call - simultaneously this
// phase's usage record, cost record, and (via a companion audit.Record)
// audit trail. Status/Error cover the fallback-exhausted case: every
// step in the route failed.
//
// Text is deliberately transient - populated on the struct Complete
// returns so the HTTP response can hand the actual generated text back
// to the caller (the entire point of a completions API), but never
// written by any Repository implementation. LLM output can be
// arbitrarily long and sensitive; this phase's usage/audit trail tracks
// what was called (model, tokens, cost, prompt key/version), not a
// permanent copy of every response body, the same "thin events, not
// full entity content" choice Phase 14's search indexer documents for
// an analogous reason.
type Completion struct {
	ID               string
	UserID           string
	AppID            string
	ModelAlias       string
	Provider         string
	Model            string
	PromptKey        string
	PromptVersion    string
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostCents        float64
	LatencyMS        int64
	FinishReason     string
	Status           CompletionStatus
	Error            string
	CreatedAt        time.Time
}

type CompletionStatus string

const (
	StatusCompleted CompletionStatus = "completed"
	StatusFailed    CompletionStatus = "failed"
)

// RouteStep is one attempt in a model alias's fallback chain - "model
// routing" and "fallback" are the same mechanism here: Steps is tried
// in order, the first success wins.
type RouteStep struct {
	Provider string
	Model    string
}

type Route struct {
	Alias string
	Steps []RouteStep
	// PricePerMillionPromptTokens/PricePerMillionCompletionTokens are
	// cents per 1,000,000 tokens - this phase's cost tracking table.
	// Ollama (this phase's one real provider) is honestly priced at 0:
	// local inference has no metered vendor cost, unlike OpenAI/
	// Anthropic/Google, which do - the pricing mechanism itself is
	// fully real and ready for a paid provider's real price to be
	// entered later.
	PricePerMillionPromptTokens     float64
	PricePerMillionCompletionTokens float64
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound       = errors.New("resource not found")
	ErrForbidden      = errors.New("not permitted to perform this AI gateway action")
	ErrUnknownRoute   = errors.New("no route registered for this model alias")
	ErrRateLimited    = errors.New("too many completion requests - try again later")
	ErrAllStepsFailed = errors.New("every provider in the fallback chain failed")
)

// Repository is the storage-agnostic boundary.
type Repository interface {
	RecordCompletion(ctx context.Context, c Completion) error
	ListCompletions(ctx context.Context, userID string, limit int) ([]Completion, error)
}
