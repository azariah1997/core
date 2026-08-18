package features

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

// Evaluate implements isEnabled(feature, context): the kill-switch first,
// then rules in ascending priority order, first full match wins. No
// matching rule means the feature evaluates off - a feature flag fails
// closed by default, the safer direction to be wrong in.
func Evaluate(feature Feature, rules []FeatureRule, ctx EvaluationContext) FeatureEvaluation {
	if !feature.Enabled {
		return FeatureEvaluation{FeatureKey: feature.Key, Enabled: false, Reason: "feature disabled (kill switch)"}
	}

	sorted := make([]FeatureRule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })

	for _, rule := range sorted {
		if matches(rule.Conditions, ctx, feature.Key) {
			return FeatureEvaluation{
				FeatureKey: feature.Key, Enabled: rule.Enabled,
				Reason:        fmt.Sprintf("matched rule %s (priority %d)", rule.ID, rule.Priority),
				MatchedRuleID: rule.ID,
			}
		}
	}
	return FeatureEvaluation{FeatureKey: feature.Key, Enabled: false, Reason: "no rule matched (default off)"}
}

// matches reports whether every condition set on the rule is satisfied.
// A rule dimension left empty imposes no constraint; a rule dimension
// that IS set but whose corresponding context value is empty fails to
// match - an explicit constraint can never be satisfied by missing
// context, the safer direction to fail in.
func matches(c RuleConditions, ctx EvaluationContext, featureKey string) bool {
	if len(c.Environments) > 0 && !contains(c.Environments, ctx.Environment) {
		return false
	}
	if len(c.UserIDs) > 0 && !contains(c.UserIDs, ctx.UserID) {
		return false
	}
	if len(c.TenantIDs) > 0 && !contains(c.TenantIDs, ctx.TenantID) {
		return false
	}
	if len(c.Platforms) > 0 && !contains(c.Platforms, ctx.Platform) {
		return false
	}
	if len(c.Countries) > 0 && !contains(c.Countries, ctx.Country) {
		return false
	}
	if c.MinVersion != "" && (ctx.Version == "" || compareVersions(ctx.Version, c.MinVersion) < 0) {
		return false
	}
	if c.MaxVersion != "" && (ctx.Version == "" || compareVersions(ctx.Version, c.MaxVersion) > 0) {
		return false
	}
	if c.Percentage != nil && !inPercentageBucket(featureKey, ctx.UserID, *c.Percentage) {
		return false
	}
	return true
}

func contains(list []string, v string) bool {
	if v == "" {
		return false
	}
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// inPercentageBucket deterministically buckets bucketKey into [0,100) via
// FNV-1a, so the same user consistently lands on the same side of a given
// rollout percentage across calls - a real rollout property, not a fresh
// coin flip each time. An empty bucketKey (no user in context) can never
// be reliably bucketed, so it's always excluded rather than guessed.
func inPercentageBucket(featureKey, bucketKey string, percentage int) bool {
	if bucketKey == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(featureKey + ":" + bucketKey))
	return int(h.Sum32()%100) < percentage
}

// compareVersions compares dotted numeric version strings segment by
// segment (e.g. "1.9" < "1.10", unlike a plain string comparison),
// treating a missing trailing segment as 0. Non-numeric segments fall
// back to a string comparison for just that segment, so a malformed
// version doesn't panic - it just compares less predictably, which is a
// reasonable degradation for something as loosely-specified as a
// caller-supplied version string.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv string
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aErr := strconv.Atoi(av)
		bi, bErr := strconv.Atoi(bv)
		if aErr == nil && bErr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
			continue
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}
