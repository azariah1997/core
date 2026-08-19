# AI Gateway

"Products must not call AI vendors directly" is the roadmap's own instruction, and the entire reason this package exists: `Service.Complete` is the *only* path to a `Provider` in this codebase - nothing outside `internal/aigateway` ever holds a provider reference or a vendor SDK.

## Provider-neutral interface

```go
type Provider interface {
    Name() string
    Complete(ctx context.Context, model string, messages []Message, maxTokens int, temperature float64) (ProviderResult, error)
}
```

is the roadmap's own "create a provider-neutral interface" instruction, satisfied literally. `aigateway/ollama` is the one real implementation this phase ships.

### Ollama: real local inference, no API key

Real infrastructure was added for this phase specifically because it's the one AI provider option the roadmap names that doesn't require a vendor account: `infra/docker/docker-compose.yml` now runs `ollama/ollama`, auto-pulling a small (~400MB) model, `qwen2.5:0.5b`, on every start (the same self-healing bootstrap precedent as Keycloak's realm import and OpenFGA's store creation - `ollama pull` is a fast no-op once the model is cached in its named volume). This is genuinely live-testable end to end, unlike OpenAI/Anthropic/Google, which this environment has no credentials for and which would otherwise mean either faking a completion (dishonest) or leaving the phase entirely unvalidated live.

`aigateway/ollama` speaks Ollama's OpenAI-compatible `/v1/chat/completions` endpoint deliberately, not Ollama's own native `/api/generate` shape - the request/response bodies are structurally the same ones a real OpenAI provider adapter would use, so adding one later is a base-URL-and-auth-header change, not a second incompatible client. Apple/Google equivalents aren't roadmap options here (this is a text-generation gateway, not app-store billing) - Anthropic and Google's Gemini API would be the natural next adapters, deferred for the same reason billing's Apple IAP/Google Play are: no real credentials to validate against honestly.

## Model routing = "don't call vendors directly," literally

Callers never name a vendor model. They call with a `ModelAlias` ("default"), resolved via a registered `Route` to an ordered list of `(Provider, Model)` steps - the same indirection that makes it possible to swap or add a backing model without any product code changing. `GET /v1/ai/models` lets a caller discover which aliases exist without ever learning a vendor's model naming.

## Fallback

`Route.Steps` is a list, not a single provider - `Complete` tries each in order, the first success wins. Confirmed via unit tests with a deliberately failing primary and a working backup (`TestCompleteFallsBackToTheNextStepOnFailure`); live validation exercises the one genuinely real provider registered today (a single-step route), so the fallback *mechanism* is proven in tests rather than against a second real vendor this environment doesn't have credentials for.

## Quotas

Reuses `platformkit/ratelimit.Limiter` for a third, unrelated purpose (Phase 21: report-spam; Phase 23: anonymous-tracking abuse; here: per-caller completion quota) - 60 completions/minute per caller, keyed the same way trustsafety's report limiter is. Confirmed live by inspecting the real Valkey key directly (`GET aigateway:complete:<userId>`) after a mix of successful and failed calls - the count precisely reflected that a validation failure (caught before the rate-limit check) didn't consume a slot, while an "unknown route" failure (caught after it) did, exactly matching `Complete`'s actual check ordering. A real token-*quantity* quota (summed from `Completion.TotalTokens`, not just a call count) is a natural extension the `Repository` this phase built already has the data for - not implemented as a live gate yet.

## Token usage, cost tracking, and audit - three views of one call

Every completion writes one `ai_completions` row (tokens, cost, latency, model, prompt key/version - never the prompt or response text itself, see `Completion`'s own doc comment for why) *and* one `audit.Record` via a new `AIGatewayAuditRecorder` adapter (`internal/api/aigateway_adapter.go`) - the same consumer-defined-interface pattern `authz.AuditRecorder` established for Phase 19, but without that one's construction-order cycle (`audit.Service` doesn't depend on `aigateway` for anything), so no two-phase wiring is needed here. Confirmed live: a completion's `id` appeared as the `resourceId` on a matching `ai.completion.succeeded` audit record, queryable via `GET /v1/audit?resourceType=ai_completion`.

Cost tracking is a real per-route price table (cents per million prompt/completion tokens), multiplying real token counts - Ollama's registered route honestly prices both at 0 (local inference has no metered vendor cost), confirmed live and by a dedicated unit test (`TestOllamaRouteIsHonestlyFree`) distinguishing "the mechanism works" from "this provider happens to cost nothing." A second unit test (`TestCostIsComputedFromRoutePricing`) proves the same arithmetic against a non-zero price, for whenever a paid provider is registered.

## Timeouts

Every provider call is wrapped in `context.WithTimeout` (`Service.attempt`), not left to whatever default the HTTP client happens to have - confirmed via a real `httptest.Server` that never responds (`TestCompleteRespectsContextCancellation` in `ollama/ollama_test.go`), proving actual client cancellation behavior, not a mocked timeout.

## Prompt/version metadata

`PromptKey`/`PromptVersion` are free-form, product-defined strings (e.g. `"greeting_v1"`) carried through the whole pipeline - the `Completion` record, the audit metadata, and the HTTP response - letting a product track which prompt template/version produced a given result, useful for prompt regression tracking or A/B testing, without this package needing to know what either string means.

## Layout

- `domain.go` - `Message`, `CompletionInput`, `Completion`, `Route`, `Repository`.
- `service.go` - `Service`, the provider/route registries, `Complete` (the one entry point), `ListCompletions`.
- `http.go` - `POST /v1/ai/completions`, `GET /v1/ai/usage`, `GET /v1/ai/models`.
- `postgres/`, `memory/` - `Repository` implementations; neither ever writes or reads `Text`.
- `ollama/` - the one real `Provider` this phase ships.

## Storage

`ai_completions` (`data/migrations/0022_aigateway.sql`) - a new table, no pre-existing scaffold.

## Not done here

OpenAI/Anthropic/Google adapters are structurally supported (the `Provider` interface, unchanged) but not implemented - this environment has no real API keys to validate them against honestly, the same reasoning Phase 22 applied to Apple IAP/Google Play. A live token-quantity quota (as opposed to today's call-count quota) and prompt-injection/output-moderation concerns are both real follow-ups outside this phase's roadmap-defined scope.
