package jobrunner

import "testing"

func TestBackoffForGrowsExponentially(t *testing.T) {
	cases := map[int]int{1: 2, 2: 4, 3: 8, 4: 16}
	for attempt, wantSeconds := range cases {
		got := backoffFor(attempt)
		if got.Seconds() != float64(wantSeconds) {
			t.Fatalf("attempt %d: expected %ds, got %v", attempt, wantSeconds, got)
		}
	}
}

func TestBackoffForCapsAtMax(t *testing.T) {
	if got := backoffFor(30); got != maxBackoff {
		t.Fatalf("expected backoff to cap at %v, got %v", maxBackoff, got)
	}
}

func TestBackoffForTreatsNonPositiveAttemptAsFirst(t *testing.T) {
	if backoffFor(0) != backoffFor(1) {
		t.Fatalf("expected attempt 0 to behave like attempt 1")
	}
}
