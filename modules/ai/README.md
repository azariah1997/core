# AI module

Owns the **ai** capability boundary. Product applications consume this module through contracts/SDKs, never by reaching into its data store.

Implemented in `backend/core-api/internal/aigateway` - "products must not call AI vendors directly" is enforced structurally: `Service.Complete` is the only path anywhere in this codebase to a `Provider`. A provider-neutral interface backs real local inference via a new Ollama container (auto-pulled model, no vendor API key) - the one live-testable provider; OpenAI/Anthropic/Google are structurally supported but unimplemented, no real credentials in this environment. Callers never name a vendor model, only a product-facing `ModelAlias`, resolved via an ordered fallback route. Includes quotas (`platformkit/ratelimit` reused a third time), real token/cost tracking, a genuine `audit.Record` per call, prompt/version metadata, and per-call timeouts. Exposed at `/v1/ai/*`. See that package's README for detail.
