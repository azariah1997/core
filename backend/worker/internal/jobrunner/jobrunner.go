// Package jobrunner is the execution half of the platform's background
// job queue - core-api's internal/jobs only ever inserts a row (fast, no
// handler ever runs inside an HTTP request per the roadmap's own "do not
// execute heavy jobs inside HTTP request handlers"); this package claims
// and actually runs jobs, in the worker process.
package jobrunner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/packages/go/platformkit/metrics"
)

const batchSize = 10

// Handler does the actual work for one job type. Returning an error
// means the attempt failed - Runner decides whether that's retried
// (with backoff) or sent to the dead letter based on the job's
// MaxAttempts, the caller's handler never needs to know about retry
// policy at all.
type Handler func(ctx context.Context, payload map[string]any) error

type Runner struct {
	pool     *pgxpool.Pool
	handlers map[string]Handler
	logger   *slog.Logger
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Runner {
	return &Runner{pool: pool, handlers: map[string]Handler{}, logger: logger}
}

func (r *Runner) Register(jobType string, h Handler) {
	r.handlers[jobType] = h
}

type claimedJob struct {
	id                 string
	jobType            string
	payload            map[string]any
	attempts           int
	maxAttempts        int
	recurrenceInterval *int // seconds; nil = one-shot
}

// PollOnce claims due jobs, runs each one, and records the outcome.
// Claiming (SELECT ... FOR UPDATE SKIP LOCKED, then UPDATE to 'running')
// happens in one short transaction; execution happens with no
// transaction open at all - unlike Phase 14's search indexer, a job's
// handler is explicitly allowed to be slow (a webhook call, for
// instance), and holding a Postgres row lock for the duration of that
// would be exactly the kind of "heavy work" the roadmap says to keep out
// of request-critical paths. Each job's outcome is then recorded in its
// own short transaction.
func (r *Runner) PollOnce(ctx context.Context) (int, error) {
	claimed, err := r.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, j := range claimed {
		r.runOne(ctx, j)
	}
	return len(claimed), nil
}

func (r *Runner) claim(ctx context.Context) ([]claimedJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT id, type, payload, attempts, max_attempts, recurrence_interval_seconds
		 FROM jobs WHERE status = 'scheduled' AND run_at <= now()
		 ORDER BY run_at LIMIT $1 FOR UPDATE SKIP LOCKED`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("select due jobs: %w", err)
	}

	var claimed []claimedJob
	for rows.Next() {
		var j claimedJob
		if err := rows.Scan(&j.id, &j.jobType, &j.payload, &j.attempts, &j.maxAttempts, &j.recurrenceInterval); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan due job: %w", err)
		}
		claimed = append(claimed, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due jobs: %w", err)
	}

	for _, j := range claimed {
		if _, err := tx.Exec(ctx, `UPDATE jobs SET status = 'running', locked_at = now() WHERE id = $1`, j.id); err != nil {
			return nil, fmt.Errorf("claim job %s: %w", j.id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}
	return claimed, nil
}

func (r *Runner) runOne(ctx context.Context, j claimedJob) {
	started := time.Now()
	handler, ok := r.handlers[j.jobType]
	var execErr error
	if !ok {
		execErr = fmt.Errorf("no handler registered for job type %q", j.jobType)
	} else {
		execErr = handler(ctx, j.payload)
	}
	if err := r.recordResult(ctx, j, execErr, started); err != nil {
		r.logger.Error("jobrunner: failed to record job result", "error", err, "jobId", j.id, "jobType", j.jobType)
	}
}

func (r *Runner) recordResult(ctx context.Context, j claimedJob, execErr error, started time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin result tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attemptNumber := j.attempts + 1
	status, errMsg := "succeeded", ""
	if execErr != nil {
		status, errMsg = "failed", execErr.Error()
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO job_attempts (job_id, attempt_number, status, error, started_at, finished_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		j.id, attemptNumber, status, nullIfEmpty(errMsg), started,
	); err != nil {
		return fmt.Errorf("insert job attempt: %w", err)
	}

	switch {
	case execErr == nil && j.recurrenceInterval != nil:
		// Recurring and this run succeeded: reschedule the same row for
		// its next occurrence and reset the attempt counter - a fresh
		// retry budget for the next cycle.
		if _, err := tx.Exec(ctx,
			`UPDATE jobs SET status = 'scheduled', run_at = now() + make_interval(secs => $1), attempts = 0, updated_at = now() WHERE id = $2`,
			*j.recurrenceInterval, j.id,
		); err != nil {
			return fmt.Errorf("reschedule recurring job: %w", err)
		}

	case execErr == nil:
		if _, err := tx.Exec(ctx,
			`UPDATE jobs SET status = 'succeeded', attempts = $1, updated_at = now() WHERE id = $2`,
			attemptNumber, j.id,
		); err != nil {
			return fmt.Errorf("mark job succeeded: %w", err)
		}

	case attemptNumber >= j.maxAttempts:
		if _, err := tx.Exec(ctx,
			`UPDATE jobs SET status = 'dead_letter', attempts = $1, updated_at = now() WHERE id = $2`,
			attemptNumber, j.id,
		); err != nil {
			return fmt.Errorf("mark job dead-lettered: %w", err)
		}

	default:
		if _, err := tx.Exec(ctx,
			`UPDATE jobs SET status = 'scheduled', attempts = $1, run_at = now() + $2, updated_at = now() WHERE id = $3`,
			attemptNumber, backoffFor(attemptNumber), j.id,
		); err != nil {
			return fmt.Errorf("reschedule failed job for retry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit result tx: %w", err)
	}
	if execErr != nil && attemptNumber >= j.maxAttempts {
		// Only a real, terminal dead-letter counts as a "job failure" for
		// this metric - a retryable attempt failure (the default branch
		// above) is expected, routine behavior, not an alertable one.
		metrics.IncJobFailure(j.jobType)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
