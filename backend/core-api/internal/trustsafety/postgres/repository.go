// Package postgres is the PostgreSQL-backed trustsafety.Repository.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/trustsafety"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Mute(ctx context.Context, m trustsafety.Mute) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO mutes (id, muter_user_id, muted_user_id, created_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (muter_user_id, muted_user_id) DO NOTHING`,
		m.ID, m.MuterUserID, m.MutedUserID, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("mute: %w", err)
	}
	return nil
}

func (r *Repository) Unmute(ctx context.Context, muterUserID, mutedUserID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM mutes WHERE muter_user_id = $1 AND muted_user_id = $2`, muterUserID, mutedUserID)
	if err != nil {
		return fmt.Errorf("unmute: %w", err)
	}
	return nil
}

func (r *Repository) ListMutes(ctx context.Context, muterUserID string) ([]trustsafety.Mute, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, muter_user_id, muted_user_id, created_at FROM mutes WHERE muter_user_id = $1 ORDER BY created_at DESC`, muterUserID)
	if err != nil {
		return nil, fmt.Errorf("list mutes: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.Mute
	for rows.Next() {
		var m trustsafety.Mute
		if err := rows.Scan(&m.ID, &m.MuterUserID, &m.MutedUserID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mute: %w", err)
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

const caseColumns = `id, resource_type, resource_id, status, coalesce(assigned_to::text, ''), coalesce(resolution, ''), report_count, created_at, resolved_at`

func scanCase(row interface{ Scan(...any) error }) (trustsafety.ModerationCase, error) {
	var c trustsafety.ModerationCase
	err := row.Scan(&c.ID, &c.ResourceType, &c.ResourceID, &c.Status, &c.AssignedTo, &c.Resolution, &c.ReportCount, &c.CreatedAt, &c.ResolvedAt)
	return c, err
}

// OpenOrAttachCase relies on the migration's partial unique index
// (resource_type, resource_id) WHERE status IN ('open','in_review') to
// atomically create-or-increment in one statement - no separate
// check-then-insert race to worry about, the same INSERT ... ON
// CONFLICT idempotency technique Phase 6's OpenFGA grant/revoke and
// Phase 11's reactions already use.
func (r *Repository) OpenOrAttachCase(ctx context.Context, resourceType, resourceID string) (trustsafety.ModerationCase, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO moderation_cases (resource_type, resource_id, status, report_count)
		 VALUES ($1, $2, 'open', 1)
		 ON CONFLICT (resource_type, resource_id) WHERE status IN ('open', 'in_review')
		 DO UPDATE SET report_count = moderation_cases.report_count + 1
		 RETURNING `+caseColumns,
		resourceType, resourceID)
	c, err := scanCase(row)
	if err != nil {
		return trustsafety.ModerationCase{}, fmt.Errorf("open or attach case: %w", err)
	}
	return c, nil
}

func (r *Repository) GetCase(ctx context.Context, id string) (trustsafety.ModerationCase, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+caseColumns+` FROM moderation_cases WHERE id = $1`, id)
	c, err := scanCase(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
		}
		return trustsafety.ModerationCase{}, fmt.Errorf("get case: %w", err)
	}
	return c, nil
}

func (r *Repository) ListCases(ctx context.Context, status trustsafety.CaseStatus) ([]trustsafety.ModerationCase, error) {
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = r.pool.Query(ctx, `SELECT `+caseColumns+` FROM moderation_cases ORDER BY created_at DESC`)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT `+caseColumns+` FROM moderation_cases WHERE status = $1 ORDER BY created_at DESC`, status)
	}
	if err != nil {
		return nil, fmt.Errorf("list cases: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.ModerationCase
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, fmt.Errorf("scan case: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (r *Repository) ResolveCase(ctx context.Context, id string, in trustsafety.ResolveCaseInput) (trustsafety.ModerationCase, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE moderation_cases SET status = $2, resolution = $3, resolved_at = now() WHERE id = $1 RETURNING `+caseColumns,
		id, in.Status, nullIfEmpty(in.Resolution))
	c, err := scanCase(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
		}
		return trustsafety.ModerationCase{}, fmt.Errorf("resolve case: %w", err)
	}
	return c, nil
}

func (r *Repository) AssignCase(ctx context.Context, id, assigneeUserID string) (trustsafety.ModerationCase, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE moderation_cases SET assigned_to = $2, status = 'in_review' WHERE id = $1 RETURNING `+caseColumns,
		id, assigneeUserID)
	c, err := scanCase(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.ModerationCase{}, trustsafety.ErrNotFound
		}
		return trustsafety.ModerationCase{}, fmt.Errorf("assign case: %w", err)
	}
	return c, nil
}

func (r *Repository) CreateReport(ctx context.Context, rep trustsafety.Report) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO reports (id, reporter_user_id, resource_type, resource_id, reason, details, case_id, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		rep.ID, rep.ReporterUserID, rep.ResourceType, rep.ResourceID, rep.Reason, nullIfEmpty(rep.Details), rep.CaseID, rep.Status, rep.CreatedAt)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	return nil
}

const reportColumns = `id, reporter_user_id, resource_type, resource_id, reason, coalesce(details, ''), case_id, status, created_at`

func (r *Repository) GetReport(ctx context.Context, id string) (trustsafety.Report, error) {
	var rep trustsafety.Report
	err := r.pool.QueryRow(ctx, `SELECT `+reportColumns+` FROM reports WHERE id = $1`, id).Scan(
		&rep.ID, &rep.ReporterUserID, &rep.ResourceType, &rep.ResourceID, &rep.Reason, &rep.Details, &rep.CaseID, &rep.Status, &rep.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Report{}, trustsafety.ErrNotFound
		}
		return trustsafety.Report{}, fmt.Errorf("get report: %w", err)
	}
	return rep, nil
}

func (r *Repository) ListReports(ctx context.Context, caseID string) ([]trustsafety.Report, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+reportColumns+` FROM reports WHERE case_id = $1 ORDER BY created_at DESC`, caseID)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.Report
	for rows.Next() {
		var rep trustsafety.Report
		if err := rows.Scan(&rep.ID, &rep.ReporterUserID, &rep.ResourceType, &rep.ResourceID, &rep.Reason, &rep.Details, &rep.CaseID, &rep.Status, &rep.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		list = append(list, rep)
	}
	return list, rows.Err()
}

const suspensionColumns = `id, user_id, reason, coalesce(case_id::text, ''), issued_by, starts_at, ends_at, lifted_at, coalesce(lifted_by::text, ''), created_at`

func scanSuspension(row interface{ Scan(...any) error }) (trustsafety.Suspension, error) {
	var s trustsafety.Suspension
	err := row.Scan(&s.ID, &s.UserID, &s.Reason, &s.CaseID, &s.IssuedBy, &s.StartsAt, &s.EndsAt, &s.LiftedAt, &s.LiftedBy, &s.CreatedAt)
	return s, err
}

func (r *Repository) CreateSuspension(ctx context.Context, s trustsafety.Suspension) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO suspensions (id, user_id, reason, case_id, issued_by, starts_at, ends_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.ID, s.UserID, s.Reason, nullIfEmpty(s.CaseID), s.IssuedBy, s.StartsAt, s.EndsAt, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("create suspension: %w", err)
	}
	return nil
}

func (r *Repository) GetSuspension(ctx context.Context, id string) (trustsafety.Suspension, error) {
	s, err := scanSuspension(r.pool.QueryRow(ctx, `SELECT `+suspensionColumns+` FROM suspensions WHERE id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Suspension{}, trustsafety.ErrNotFound
		}
		return trustsafety.Suspension{}, fmt.Errorf("get suspension: %w", err)
	}
	return s, nil
}

func (r *Repository) ListSuspensions(ctx context.Context, userID string) ([]trustsafety.Suspension, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+suspensionColumns+` FROM suspensions WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list suspensions: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.Suspension
	for rows.Next() {
		s, err := scanSuspension(rows)
		if err != nil {
			return nil, fmt.Errorf("scan suspension: %w", err)
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *Repository) LiftSuspension(ctx context.Context, id, liftedBy string) (trustsafety.Suspension, error) {
	s, err := scanSuspension(r.pool.QueryRow(ctx,
		`UPDATE suspensions SET lifted_at = now(), lifted_by = $2 WHERE id = $1 RETURNING `+suspensionColumns,
		id, liftedBy))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Suspension{}, trustsafety.ErrNotFound
		}
		return trustsafety.Suspension{}, fmt.Errorf("lift suspension: %w", err)
	}
	return s, nil
}

const banColumns = `id, user_id, reason, coalesce(case_id::text, ''), issued_by, lifted_at, coalesce(lifted_by::text, ''), created_at`

func scanBan(row interface{ Scan(...any) error }) (trustsafety.Ban, error) {
	var b trustsafety.Ban
	err := row.Scan(&b.ID, &b.UserID, &b.Reason, &b.CaseID, &b.IssuedBy, &b.LiftedAt, &b.LiftedBy, &b.CreatedAt)
	return b, err
}

func (r *Repository) CreateBan(ctx context.Context, b trustsafety.Ban) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO bans (id, user_id, reason, case_id, issued_by, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		b.ID, b.UserID, b.Reason, nullIfEmpty(b.CaseID), b.IssuedBy, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("create ban: %w", err)
	}
	return nil
}

func (r *Repository) GetBan(ctx context.Context, id string) (trustsafety.Ban, error) {
	b, err := scanBan(r.pool.QueryRow(ctx, `SELECT `+banColumns+` FROM bans WHERE id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Ban{}, trustsafety.ErrNotFound
		}
		return trustsafety.Ban{}, fmt.Errorf("get ban: %w", err)
	}
	return b, nil
}

func (r *Repository) ListBans(ctx context.Context, userID string) ([]trustsafety.Ban, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+banColumns+` FROM bans WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list bans: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.Ban
	for rows.Next() {
		b, err := scanBan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ban: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *Repository) LiftBan(ctx context.Context, id, liftedBy string) (trustsafety.Ban, error) {
	b, err := scanBan(r.pool.QueryRow(ctx,
		`UPDATE bans SET lifted_at = now(), lifted_by = $2 WHERE id = $1 RETURNING `+banColumns,
		id, liftedBy))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Ban{}, trustsafety.ErrNotFound
		}
		return trustsafety.Ban{}, fmt.Errorf("lift ban: %w", err)
	}
	return b, nil
}

const appealColumns = `id, user_id, target_type, target_id, reason, status, coalesce(reviewed_by::text, ''), created_at, reviewed_at`

func scanAppeal(row interface{ Scan(...any) error }) (trustsafety.Appeal, error) {
	var a trustsafety.Appeal
	err := row.Scan(&a.ID, &a.UserID, &a.TargetType, &a.TargetID, &a.Reason, &a.Status, &a.ReviewedBy, &a.CreatedAt, &a.ReviewedAt)
	return a, err
}

func (r *Repository) CreateAppeal(ctx context.Context, a trustsafety.Appeal) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO appeals (id, user_id, target_type, target_id, reason, status, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.ID, a.UserID, a.TargetType, a.TargetID, a.Reason, a.Status, a.CreatedAt)
	if err != nil {
		return fmt.Errorf("create appeal: %w", err)
	}
	return nil
}

func (r *Repository) GetAppeal(ctx context.Context, id string) (trustsafety.Appeal, error) {
	a, err := scanAppeal(r.pool.QueryRow(ctx, `SELECT `+appealColumns+` FROM appeals WHERE id = $1`, id))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Appeal{}, trustsafety.ErrNotFound
		}
		return trustsafety.Appeal{}, fmt.Errorf("get appeal: %w", err)
	}
	return a, nil
}

func (r *Repository) ListAppeals(ctx context.Context, userID string) ([]trustsafety.Appeal, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+appealColumns+` FROM appeals WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list appeals: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.Appeal
	for rows.Next() {
		a, err := scanAppeal(rows)
		if err != nil {
			return nil, fmt.Errorf("scan appeal: %w", err)
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (r *Repository) ReviewAppeal(ctx context.Context, id string, status trustsafety.AppealStatus, reviewedBy string) (trustsafety.Appeal, error) {
	a, err := scanAppeal(r.pool.QueryRow(ctx,
		`UPDATE appeals SET status = $2, reviewed_by = $3, reviewed_at = now() WHERE id = $1 RETURNING `+appealColumns,
		id, status, reviewedBy))
	if err != nil {
		if err == pgx.ErrNoRows {
			return trustsafety.Appeal{}, trustsafety.ErrNotFound
		}
		return trustsafety.Appeal{}, fmt.Errorf("review appeal: %w", err)
	}
	return a, nil
}

func (r *Repository) RecordSignal(ctx context.Context, sig trustsafety.AbuseSignal) error {
	metadata, err := json.Marshal(sig.Metadata)
	if err != nil {
		return fmt.Errorf("marshal signal metadata: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO abuse_signals (id, resource_type, resource_id, signal_type, severity, metadata, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sig.ID, sig.ResourceType, sig.ResourceID, sig.SignalType, sig.Severity, metadata, sig.RecordedAt)
	if err != nil {
		return fmt.Errorf("record signal: %w", err)
	}
	return nil
}

func (r *Repository) ListSignals(ctx context.Context, resourceType, resourceID string) ([]trustsafety.AbuseSignal, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, resource_type, resource_id, signal_type, severity, metadata, recorded_at
		 FROM abuse_signals WHERE resource_type = $1 AND resource_id = $2 ORDER BY recorded_at DESC`,
		resourceType, resourceID)
	if err != nil {
		return nil, fmt.Errorf("list signals: %w", err)
	}
	defer rows.Close()
	var list []trustsafety.AbuseSignal
	for rows.Next() {
		var s trustsafety.AbuseSignal
		var metadata []byte
		if err := rows.Scan(&s.ID, &s.ResourceType, &s.ResourceID, &s.SignalType, &s.Severity, &metadata, &s.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &s.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal signal metadata: %w", err)
			}
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
