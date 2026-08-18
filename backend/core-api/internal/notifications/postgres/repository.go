// Package postgres is the PostgreSQL-backed notifications.Repository. It
// splits across two tables with a naming mismatch worth remembering: the
// pre-existing scaffold table "notifications" holds one channel's
// delivery attempt (NotificationDelivery), while the logical, potentially
// multi-channel request (Notification) lives in the new
// notification_requests table added by 0011_notifications.sql.
package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const requestColumns = "id, app_id, user_id, category, title, body, data, channels, created_at"

// provider_ref/error are nullable columns; COALESCE keeps
// NotificationDelivery.ProviderRef/Error as plain (never-nil) strings
// rather than requiring every caller to scan through a *string.
const deliveryColumns = "id, notification_request_id, channel, status, COALESCE(provider_ref,''), COALESCE(error,''), attempts, created_at, updated_at, delivered_at"

func (r *Repository) CreateNotification(ctx context.Context, in notifications.SendInput, title, body string) (notifications.Notification, error) {
	data, err := marshalJSON(in.Data)
	if err != nil {
		return notifications.Notification{}, err
	}
	channels := make([]string, len(in.Channels))
	for i, c := range in.Channels {
		channels[i] = string(c)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return notifications.Notification{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n notifications.Notification
	var channelsRaw []string
	err = tx.QueryRow(ctx,
		`INSERT INTO notification_requests (app_id, user_id, category, title, body, data, channels)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+requestColumns,
		in.AppID, in.UserID, in.Category, title, body, data, channels,
	).Scan(&n.ID, &n.AppID, &n.UserID, &n.Category, &n.Title, &n.Body, &n.Data, &channelsRaw, &n.CreatedAt)
	if err != nil {
		return notifications.Notification{}, fmt.Errorf("insert notification request: %w", err)
	}
	n.Channels = toChannels(channelsRaw)

	if err := insertOutboxEvent(ctx, tx, "notification.requested", n.ID, map[string]any{
		"id": n.ID, "userId": n.UserID, "category": n.Category, "channels": channels,
	}); err != nil {
		return notifications.Notification{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return notifications.Notification{}, fmt.Errorf("commit tx: %w", err)
	}
	return n, nil
}

func (r *Repository) GetNotification(ctx context.Context, id string) (notifications.Notification, error) {
	var n notifications.Notification
	var channelsRaw []string
	err := r.pool.QueryRow(ctx, `SELECT `+requestColumns+` FROM notification_requests WHERE id = $1`, id).
		Scan(&n.ID, &n.AppID, &n.UserID, &n.Category, &n.Title, &n.Body, &n.Data, &channelsRaw, &n.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.Notification{}, notifications.ErrNotFound
		}
		return notifications.Notification{}, fmt.Errorf("get notification: %w", err)
	}
	n.Channels = toChannels(channelsRaw)
	return n, nil
}

func (r *Repository) ListNotificationsForUser(ctx context.Context, userID string, params notifications.ListParams) (notifications.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+requestColumns+` FROM notification_requests WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`,
			userID, params.Limit+1)
	} else {
		beforeCreated, beforeID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return notifications.ListResult{}, &notifications.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+requestColumns+` FROM notification_requests
			 WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			userID, beforeCreated, beforeID, params.Limit+1)
	}
	if err != nil {
		return notifications.ListResult{}, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var items []notifications.Notification
	for rows.Next() {
		var n notifications.Notification
		var channelsRaw []string
		if err := rows.Scan(&n.ID, &n.AppID, &n.UserID, &n.Category, &n.Title, &n.Body, &n.Data, &channelsRaw, &n.CreatedAt); err != nil {
			return notifications.ListResult{}, fmt.Errorf("scan notification: %w", err)
		}
		n.Channels = toChannels(channelsRaw)
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return notifications.ListResult{}, fmt.Errorf("iterate notifications: %w", err)
	}

	result := notifications.ListResult{Items: items}
	if len(items) > params.Limit {
		last := items[params.Limit-1]
		result.Items = items[:params.Limit]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return result, nil
}

// CreateDelivery denormalizes app_id/user_id/category/payload from the
// parent Notification onto the delivery row - the pre-existing
// "notifications" table already had those columns NOT NULL, and keeping
// each delivery row self-contained matches what was already there rather
// than requiring a join for anything that reads this table directly.
func (r *Repository) CreateDelivery(ctx context.Context, n notifications.Notification, channel notifications.Channel) (notifications.NotificationDelivery, error) {
	payload, err := marshalJSON(map[string]any{"title": n.Title, "body": n.Body, "data": n.Data})
	if err != nil {
		return notifications.NotificationDelivery{}, err
	}
	var d notifications.NotificationDelivery
	err = r.pool.QueryRow(ctx,
		`INSERT INTO notifications (notification_request_id, app_id, user_id, category, channel, payload, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending') RETURNING `+deliveryColumns,
		n.ID, n.AppID, n.UserID, n.Category, string(channel), payload,
	).Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.ProviderRef, &d.Error, &d.Attempts, &d.CreatedAt, &d.UpdatedAt, &d.DeliveredAt)
	if err != nil {
		return notifications.NotificationDelivery{}, fmt.Errorf("insert delivery: %w", err)
	}
	return d, nil
}

// UpdateDeliveryResult records a send attempt's outcome and, in the same
// transaction, emits the matching outbox event - notification.sent,
// notification.delivered or notification.failed. Deferred (quiet hours)
// and skipped (preference opt-out) outcomes intentionally emit no event:
// the roadmap names only requested/sent/delivered/failed.
func (r *Repository) UpdateDeliveryResult(ctx context.Context, deliveryID string, status notifications.Status, providerRef, errMsg string) (notifications.NotificationDelivery, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return notifications.NotificationDelivery{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var deliveredAtExpr string
	if status == notifications.StatusDelivered {
		deliveredAtExpr = "now()"
	} else {
		deliveredAtExpr = "delivered_at"
	}
	var d notifications.NotificationDelivery
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`UPDATE notifications SET status = $1, provider_ref = $2, error = $3,
		 attempts = attempts + 1, updated_at = now(), delivered_at = %s
		 WHERE id = $4 RETURNING `+deliveryColumns, deliveredAtExpr),
		string(status), nullIfEmpty(providerRef), nullIfEmpty(errMsg), deliveryID,
	).Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.ProviderRef, &d.Error, &d.Attempts, &d.CreatedAt, &d.UpdatedAt, &d.DeliveredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.NotificationDelivery{}, notifications.ErrDeliveryNotFound
		}
		return notifications.NotificationDelivery{}, fmt.Errorf("update delivery: %w", err)
	}

	eventType := ""
	switch status {
	case notifications.StatusSent:
		eventType = "notification.sent"
	case notifications.StatusDelivered:
		eventType = "notification.delivered"
	case notifications.StatusFailed:
		eventType = "notification.failed"
	}
	if eventType != "" {
		if err := insertOutboxEvent(ctx, tx, eventType, d.NotificationID, map[string]any{
			"deliveryId": d.ID, "notificationId": d.NotificationID, "channel": d.Channel, "status": d.Status,
		}); err != nil {
			return notifications.NotificationDelivery{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return notifications.NotificationDelivery{}, fmt.Errorf("commit tx: %w", err)
	}
	return d, nil
}

func (r *Repository) GetDelivery(ctx context.Context, id string) (notifications.NotificationDelivery, error) {
	var d notifications.NotificationDelivery
	err := r.pool.QueryRow(ctx, `SELECT `+deliveryColumns+` FROM notifications WHERE id = $1`, id).
		Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.ProviderRef, &d.Error, &d.Attempts, &d.CreatedAt, &d.UpdatedAt, &d.DeliveredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.NotificationDelivery{}, notifications.ErrDeliveryNotFound
		}
		return notifications.NotificationDelivery{}, fmt.Errorf("get delivery: %w", err)
	}
	return d, nil
}

func (r *Repository) ListDeliveries(ctx context.Context, notificationID string) ([]notifications.NotificationDelivery, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+deliveryColumns+` FROM notifications WHERE notification_request_id = $1 ORDER BY created_at`, notificationID)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var list []notifications.NotificationDelivery
	for rows.Next() {
		var d notifications.NotificationDelivery
		if err := rows.Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.ProviderRef, &d.Error, &d.Attempts, &d.CreatedAt, &d.UpdatedAt, &d.DeliveredAt); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deliveries: %w", err)
	}
	return list, nil
}

func (r *Repository) GetTemplate(ctx context.Context, appID, key string) (notifications.NotificationTemplate, error) {
	var t notifications.NotificationTemplate
	err := r.pool.QueryRow(ctx,
		`SELECT id, app_id, key, title_template, body_template, created_at, updated_at
		 FROM notification_templates WHERE app_id = $1 AND key = $2`,
		appID, key,
	).Scan(&t.ID, &t.AppID, &t.Key, &t.TitleTemplate, &t.BodyTemplate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.NotificationTemplate{}, notifications.ErrTemplateNotFound
		}
		return notifications.NotificationTemplate{}, fmt.Errorf("get template: %w", err)
	}
	return t, nil
}

func (r *Repository) CreateTemplate(ctx context.Context, in notifications.CreateTemplateInput) (notifications.NotificationTemplate, error) {
	var t notifications.NotificationTemplate
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notification_templates (app_id, key, title_template, body_template)
		 VALUES ($1, $2, $3, $4) RETURNING id, app_id, key, title_template, body_template, created_at, updated_at`,
		in.AppID, in.Key, in.TitleTemplate, in.BodyTemplate,
	).Scan(&t.ID, &t.AppID, &t.Key, &t.TitleTemplate, &t.BodyTemplate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return notifications.NotificationTemplate{}, notifications.ErrTemplateKeyTaken
		}
		return notifications.NotificationTemplate{}, fmt.Errorf("insert template: %w", err)
	}
	return t, nil
}

func (r *Repository) UpdateTemplate(ctx context.Context, appID, key string, in notifications.UpdateTemplateInput) (notifications.NotificationTemplate, error) {
	setClauses := []string{"updated_at = now()"}
	args := []any{}
	if in.TitleTemplate != nil {
		args = append(args, *in.TitleTemplate)
		setClauses = append(setClauses, fmt.Sprintf("title_template = $%d", len(args)))
	}
	if in.BodyTemplate != nil {
		args = append(args, *in.BodyTemplate)
		setClauses = append(setClauses, fmt.Sprintf("body_template = $%d", len(args)))
	}
	args = append(args, appID, key)
	query := fmt.Sprintf(`UPDATE notification_templates SET %s WHERE app_id = $%d AND key = $%d
		 RETURNING id, app_id, key, title_template, body_template, created_at, updated_at`,
		strings.Join(setClauses, ", "), len(args)-1, len(args))

	var t notifications.NotificationTemplate
	err := r.pool.QueryRow(ctx, query, args...).
		Scan(&t.ID, &t.AppID, &t.Key, &t.TitleTemplate, &t.BodyTemplate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notifications.NotificationTemplate{}, notifications.ErrTemplateNotFound
		}
		return notifications.NotificationTemplate{}, fmt.Errorf("update template: %w", err)
	}
	return t, nil
}

func (r *Repository) GetPreferences(ctx context.Context, userID, appID string) ([]notifications.NotificationPreference, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, app_id, category, channel, enabled FROM notification_preferences WHERE user_id = $1 AND app_id = $2`,
		userID, appID)
	if err != nil {
		return nil, fmt.Errorf("get preferences: %w", err)
	}
	defer rows.Close()

	var list []notifications.NotificationPreference
	for rows.Next() {
		var p notifications.NotificationPreference
		if err := rows.Scan(&p.UserID, &p.AppID, &p.Category, &p.Channel, &p.Enabled); err != nil {
			return nil, fmt.Errorf("scan preference: %w", err)
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate preferences: %w", err)
	}
	return list, nil
}

func (r *Repository) SetPreference(ctx context.Context, userID, appID string, in notifications.SetPreferenceInput) (notifications.NotificationPreference, error) {
	var p notifications.NotificationPreference
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notification_preferences (user_id, app_id, category, channel, enabled)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, app_id, category, channel) DO UPDATE SET enabled = $5
		 RETURNING user_id, app_id, category, channel, enabled`,
		userID, appID, in.Category, string(in.Channel), in.Enabled,
	).Scan(&p.UserID, &p.AppID, &p.Category, &p.Channel, &p.Enabled)
	if err != nil {
		return notifications.NotificationPreference{}, fmt.Errorf("set preference: %w", err)
	}
	return p, nil
}

func (r *Repository) GetQuietHours(ctx context.Context, userID, appID string) (notifications.QuietHours, error) {
	var q notifications.QuietHours
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, app_id, timezone, start_minute, end_minute, enabled FROM notification_quiet_hours WHERE user_id = $1 AND app_id = $2`,
		userID, appID,
	).Scan(&q.UserID, &q.AppID, &q.Timezone, &q.StartMinute, &q.EndMinute, &q.Enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No configured quiet hours is the normal state for most users,
			// not a not-found error - Active() on the zero value (Enabled:
			// false) is always false, so callers don't need to special-case it.
			return notifications.QuietHours{UserID: userID, AppID: appID, Timezone: "UTC"}, nil
		}
		return notifications.QuietHours{}, fmt.Errorf("get quiet hours: %w", err)
	}
	return q, nil
}

func (r *Repository) SetQuietHours(ctx context.Context, userID, appID string, in notifications.SetQuietHoursInput) (notifications.QuietHours, error) {
	var q notifications.QuietHours
	err := r.pool.QueryRow(ctx,
		`INSERT INTO notification_quiet_hours (user_id, app_id, timezone, start_minute, end_minute, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, app_id) DO UPDATE SET timezone = $3, start_minute = $4, end_minute = $5, enabled = $6
		 RETURNING user_id, app_id, timezone, start_minute, end_minute, enabled`,
		userID, appID, in.Timezone, in.StartMinute, in.EndMinute, in.Enabled,
	).Scan(&q.UserID, &q.AppID, &q.Timezone, &q.StartMinute, &q.EndMinute, &q.Enabled)
	if err != nil {
		return notifications.QuietHours{}, fmt.Errorf("set quiet hours: %w", err)
	}
	return q, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, eventType, aggregateID string, data map[string]any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('notification', $1, $2, 1, $3, $4)`,
		aggregateID, eventType, payload, nullIfEmpty(correlation.FromContext(ctx)),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func toChannels(raw []string) []notifications.Channel {
	out := make([]notifications.Channel, len(raw))
	for i, s := range raw {
		out[i] = notifications.Channel(s)
	}
	return out
}

func marshalJSON(m map[string]any) ([]byte, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return b, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("malformed cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}
