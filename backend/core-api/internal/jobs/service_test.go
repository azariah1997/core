package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/jobs"
	"github.com/example/core-platform/backend/core-api/internal/jobs/memory"
)

type fakeAdmin struct{ admins map[string]bool }

func (a fakeAdmin) IsPlatformAdmin(ctx context.Context, userID string) (bool, error) {
	return a.admins[userID], nil
}

func newService(admins map[string]bool) *jobs.Service {
	return jobs.NewService(memory.New(), fakeAdmin{admins: admins})
}

func TestEnqueueRequiresType(t *testing.T) {
	svc := newService(nil)
	_, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{})
	var verr *jobs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestEnqueueRejectsRunAtAndDelayTogether(t *testing.T) {
	svc := newService(nil)
	runAt := time.Now().Add(time.Hour)
	delay := 60
	_, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo", RunAt: &runAt, DelaySeconds: &delay})
	var verr *jobs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError for mutually exclusive runAt/delaySeconds, got %v", err)
	}
}

func TestEnqueueImmediateDefaultsRunAtToNow(t *testing.T) {
	svc := newService(nil)
	before := time.Now()
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if j.RunAt.Before(before) || j.RunAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("expected an immediate job's RunAt to be ~now, got %v (before=%v)", j.RunAt, before)
	}
	if j.Status != jobs.StatusScheduled {
		t.Fatalf("expected status scheduled, got %v", j.Status)
	}
}

func TestEnqueueDelayedComputesRunAtFromNow(t *testing.T) {
	svc := newService(nil)
	delay := 300
	before := time.Now()
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo", DelaySeconds: &delay})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	expectedEarliest := before.Add(299 * time.Second)
	if j.RunAt.Before(expectedEarliest) {
		t.Fatalf("expected RunAt ~5 minutes out, got %v (enqueued at %v)", j.RunAt, before)
	}
}

func TestEnqueueRecurringSetsInterval(t *testing.T) {
	svc := newService(nil)
	interval := 120
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo", RecurrenceIntervalSeconds: &interval})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if j.RecurrenceInterval == nil || *j.RecurrenceInterval != 120*time.Second {
		t.Fatalf("expected a 120s recurrence interval, got %v", j.RecurrenceInterval)
	}
}

func TestEnqueueDefaultsMaxAttempts(t *testing.T) {
	svc := newService(nil)
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if j.MaxAttempts != 5 {
		t.Fatalf("expected default max attempts of 5, got %d", j.MaxAttempts)
	}
}

func TestNonOwnerCannotGetJob(t *testing.T) {
	svc := newService(nil)
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Get(context.Background(), "stranger", j.ID); !errors.Is(err, jobs.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAdminCanGetAnyonesJob(t *testing.T) {
	svc := newService(map[string]bool{"admin": true})
	j, err := svc.Enqueue(context.Background(), "u1", jobs.EnqueueInput{Type: "echo"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := svc.Get(context.Background(), "admin", j.ID); err != nil {
		t.Fatalf("expected admin to access anyone's job, got %v", err)
	}
}

func TestListMineReturnsOnlyOwnJobs(t *testing.T) {
	svc := newService(nil)
	ctx := context.Background()
	if _, err := svc.Enqueue(ctx, "u1", jobs.EnqueueInput{Type: "echo"}); err != nil {
		t.Fatalf("enqueue u1: %v", err)
	}
	if _, err := svc.Enqueue(ctx, "u2", jobs.EnqueueInput{Type: "echo"}); err != nil {
		t.Fatalf("enqueue u2: %v", err)
	}
	result, err := svc.ListMine(ctx, "u1", jobs.ListParams{})
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected only u1's job, got %d", len(result.Items))
	}
}
