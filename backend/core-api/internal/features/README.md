# Feature Flags

"Applications ask `isEnabled(feature, context)`" is the roadmap's own framing for the entry point - `Service.Evaluate`. `Feature`/`FeatureRule`/`FeatureEvaluation` are the three constructs it names.

## Responsibilities

- `Feature` is a named, per-app flag with a master kill-switch (`Enabled`) - false always evaluates the feature off before any rule is even considered.
- `FeatureRule` targets by every dimension the roadmap names except "app" (a `Feature` is already scoped to one `AppID`, so that dimension needs no per-rule condition): environment, user, tenant, platform, country, percentage rollout, and version. Rules are evaluated in ascending `Priority` order; the first whose conditions all match wins, and what it resolves to (`Enabled` on the rule) is usually `true` but can be `false` - letting a narrow, high-priority rule explicitly exclude a segment before a later, broader rule would otherwise include it.
- `FeatureEvaluation` is the answer to `isEnabled`, with the reasoning behind it (which rule matched, or why none did). It's a **computed value, never persisted** - a flag check can happen on effectively every request in a real system, and logging every evaluation would need sampling/aggregation infrastructure this phase doesn't build. Deliberately different from `JobAttempt` or `NotificationDelivery`, where each row represents an inherently rarer, individually significant event.

## Scoping decisions

- **Percentage rollout is deterministic, not a coin flip.** `inPercentageBucket` hashes `(featureKey, EvaluationContext.UserID)` with FNV-1a, so the same user always lands on the same side of a given rollout percentage across calls - confirmed in this phase's live validation across 20 real users, each queried twice. An empty `UserID` can never be reliably bucketed and is always excluded, even from a 100% rollout rule - the safer direction to be wrong in.
- **No matching rule means the feature evaluates off.** A flag fails closed by default, same reasoning as an explicit rule constraint that the context doesn't satisfy (see `matches` in `evaluate.go`): an unmet condition is a failure to match, not an ambiguous pass.
- **Version comparison is dotted-numeric, not full semver.** `"1.10.0" > "1.9.0"` numerically (confirmed by a dedicated test - a plain string comparison gets this backwards), but there's no prerelease/build-metadata handling. Reasonable for what the roadmap actually asks for; a product needing full semver semantics would need more than this.
- **Management is platform.admin only; reads and Evaluate are open to any authenticated caller.** Flags aren't sensitive per-user data, and `Evaluate` needs the same read `GetFeature`/`ListRules` already provide - reusing Phase 6's `authz.Service` (`AdminChecker`, satisfied directly by `IsPlatformAdmin`, no adapter) rather than inventing new permission logic, the same pattern every admin-gated module in this repo already uses.
- **A rule must belong to the feature named in its URL**, checked in `Service` before any rule update/delete - otherwise a caller who can guess/enumerate a rule ID could operate on it through an unrelated feature's path. Confirmed live: deleting a real rule through the wrong feature's URL correctly 404s.

## Layout

- `domain.go` - types, validation, `Repository` interface.
- `evaluate.go` - the pure evaluation engine (`Evaluate`, `matches`, `inPercentageBucket`, `compareVersions`) - no `Repository`, no I/O, fully unit-testable in isolation.
- `service.go` - `Service`, admin gating, and the rule-ownership check.
- `http.go` - the REST surface. Takes `requireUser`, same pattern as every other module.
- `postgres/` - the production `Repository`. `RuleConditions` is stored as a single `jsonb` column (`conditions`) rather than a dozen nullable columns or join tables - every targeting dimension is independently optional, and jsonb lets a rule combine any subset of them without a schema change per new dimension.
- `memory/` - in-memory `Repository` for tests.

## Storage

`features`, `feature_rules` (`data/migrations/0015_features.sql`) - fully new tables, no pre-existing scaffold to adapt (same as `jobs` and `workflow_runs`).
