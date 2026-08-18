package features

import "testing"

func intPtr(i int) *int { return &i }

func TestEvaluateKillSwitchOverridesEverything(t *testing.T) {
	f := Feature{Key: "x", Enabled: false}
	rules := []FeatureRule{{ID: "r1", Enabled: true}} // would otherwise match unconditionally
	eval := Evaluate(f, rules, EvaluationContext{})
	if eval.Enabled {
		t.Fatalf("expected disabled feature to always evaluate false, got %+v", eval)
	}
}

func TestEvaluateNoRulesDefaultsOff(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	eval := Evaluate(f, nil, EvaluationContext{})
	if eval.Enabled {
		t.Fatalf("expected no matching rules to default off, got %+v", eval)
	}
}

func TestEvaluateFirstMatchingRuleByPriorityWins(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{
		{ID: "low-priority-broad", Priority: 10, Enabled: true},                                 // matches everything
		{ID: "high-priority-narrow", Priority: 1, Enabled: false, Conditions: RuleConditions{}}, // also matches everything, but is checked first
	}
	eval := Evaluate(f, rules, EvaluationContext{})
	if eval.MatchedRuleID != "high-priority-narrow" || eval.Enabled {
		t.Fatalf("expected the lower-priority-number rule to win, got %+v", eval)
	}
}

func TestEvaluateEnvironmentCondition(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{Environments: []string{"production"}}}}

	if eval := Evaluate(f, rules, EvaluationContext{Environment: "production"}); !eval.Enabled {
		t.Fatalf("expected production to match, got %+v", eval)
	}
	if eval := Evaluate(f, rules, EvaluationContext{Environment: "staging"}); eval.Enabled {
		t.Fatalf("expected staging to not match, got %+v", eval)
	}
	if eval := Evaluate(f, rules, EvaluationContext{}); eval.Enabled {
		t.Fatalf("expected empty context to not satisfy an explicit environment constraint, got %+v", eval)
	}
}

func TestEvaluateUserIDAllowlist(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{UserIDs: []string{"u1", "u2"}}}}
	if eval := Evaluate(f, rules, EvaluationContext{UserID: "u2"}); !eval.Enabled {
		t.Fatalf("expected u2 to be allowlisted, got %+v", eval)
	}
	if eval := Evaluate(f, rules, EvaluationContext{UserID: "u3"}); eval.Enabled {
		t.Fatalf("expected u3 to not be allowlisted, got %+v", eval)
	}
}

func TestEvaluateVersionRange(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{MinVersion: "2.0.0", MaxVersion: "3.0.0"}}}

	cases := map[string]bool{"1.9.9": false, "2.0.0": true, "2.5.0": true, "3.0.0": true, "3.0.1": false}
	for version, want := range cases {
		eval := Evaluate(f, rules, EvaluationContext{Version: version})
		if eval.Enabled != want {
			t.Fatalf("version %s: expected enabled=%v, got %+v", version, want, eval)
		}
	}
}

func TestCompareVersionsHandlesMultiDigitSegments(t *testing.T) {
	// A plain string comparison would say "1.10.0" < "1.9.0" (lexicographic) - version comparison must not.
	if compareVersions("1.10.0", "1.9.0") <= 0 {
		t.Fatal("expected 1.10.0 > 1.9.0 numerically, not lexicographically")
	}
}

func TestEvaluatePercentageRolloutIsDeterministic(t *testing.T) {
	f := Feature{Key: "my-feature", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{Percentage: intPtr(50)}}}

	first := Evaluate(f, rules, EvaluationContext{UserID: "stable-user-123"})
	for i := 0; i < 5; i++ {
		again := Evaluate(f, rules, EvaluationContext{UserID: "stable-user-123"})
		if again.Enabled != first.Enabled {
			t.Fatalf("expected percentage rollout to be deterministic for the same user, got %v then %v", first.Enabled, again.Enabled)
		}
	}
}

func TestEvaluatePercentageRolloutSplitsPopulation(t *testing.T) {
	f := Feature{Key: "my-feature", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{Percentage: intPtr(50)}}}

	in, out := 0, 0
	for i := 0; i < 200; i++ {
		userID := "user-" + string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune('A'+i%26))
		if Evaluate(f, rules, EvaluationContext{UserID: userID}).Enabled {
			in++
		} else {
			out++
		}
	}
	if in == 0 || out == 0 {
		t.Fatalf("expected a 50%% rollout to produce both in and out results across 200 users, got in=%d out=%d", in, out)
	}
}

func TestEvaluatePercentageRolloutExcludesAnonymousContext(t *testing.T) {
	f := Feature{Key: "my-feature", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{Percentage: intPtr(100)}}} // even 100% can't include an unbucketable context
	eval := Evaluate(f, rules, EvaluationContext{})
	if eval.Enabled {
		t.Fatalf("expected an empty UserID to never be reliably bucketed, got %+v", eval)
	}
}

func TestEvaluateCombinedConditionsAllMustMatch(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{{ID: "r1", Enabled: true, Conditions: RuleConditions{
		Environments: []string{"production"}, Countries: []string{"US"},
	}}}
	if eval := Evaluate(f, rules, EvaluationContext{Environment: "production", Country: "US"}); !eval.Enabled {
		t.Fatalf("expected both conditions satisfied to match, got %+v", eval)
	}
	if eval := Evaluate(f, rules, EvaluationContext{Environment: "production", Country: "CA"}); eval.Enabled {
		t.Fatalf("expected a partial match (wrong country) to fail, got %+v", eval)
	}
}

func TestEvaluateRuleCanExplicitlyDisableBeforeABroaderRule(t *testing.T) {
	f := Feature{Key: "x", Enabled: true}
	rules := []FeatureRule{
		{ID: "exclude-beta-testers", Priority: 1, Enabled: false, Conditions: RuleConditions{UserIDs: []string{"beta-tester"}}},
		{ID: "everyone-else", Priority: 2, Enabled: true},
	}
	if eval := Evaluate(f, rules, EvaluationContext{UserID: "beta-tester"}); eval.Enabled || eval.MatchedRuleID != "exclude-beta-testers" {
		t.Fatalf("expected the exclusion rule to win for the beta tester, got %+v", eval)
	}
	if eval := Evaluate(f, rules, EvaluationContext{UserID: "regular-user"}); !eval.Enabled || eval.MatchedRuleID != "everyone-else" {
		t.Fatalf("expected the broader rule to win for everyone else, got %+v", eval)
	}
}
