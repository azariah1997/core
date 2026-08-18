// Package postgres is the PostgreSQL-backed messaging.Repository. It
// builds on the pre-existing conversations/conversation_members/messages
// tables from the original scaffold (0001_core.sql) rather than replacing
// them - message_type/payload are that scaffold's column names, kept as-is
// the same way relationships kept user_a/user_b - and adds the tables
// 0010_messaging.sql introduces for attachments, deliveries, read state
// and reactions.
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

	"github.com/example/core-platform/backend/core-api/internal/messaging"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const conversationColumns = "id, app_id, type, metadata, created_at, updated_at"
const memberColumns = "conversation_id, user_id, joined_at"
const messageColumns = "id, conversation_id, sender_id, message_type, payload, created_at"

func (r *Repository) CreateConversation(ctx context.Context, creatorUserID string, in messaging.CreateConversationInput) (messaging.Conversation, error) {
	metadata, err := marshalJSON(in.Metadata)
	if err != nil {
		return messaging.Conversation{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return messaging.Conversation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c messaging.Conversation
	err = tx.QueryRow(ctx,
		`INSERT INTO conversations (app_id, type, metadata) VALUES ($1, $2, $3) RETURNING `+conversationColumns,
		in.AppID, string(in.Type), metadata,
	).Scan(&c.ID, &c.AppID, &c.Type, &c.Metadata, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return messaging.Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}

	seen := map[string]bool{}
	for _, userID := range in.MemberUserIDs {
		if seen[userID] {
			continue
		}
		seen[userID] = true
		if _, err := tx.Exec(ctx,
			`INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			c.ID, userID,
		); err != nil {
			return messaging.Conversation{}, fmt.Errorf("insert member: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return messaging.Conversation{}, fmt.Errorf("commit tx: %w", err)
	}
	_ = creatorUserID // already included in MemberUserIDs by the caller (http.go); kept as a parameter to mirror groups.Repository.Create's signature
	return c, nil
}

// FindDirectBetween returns the existing direct conversation between
// exactly these two users in this app, if any - the application-level
// half of direct-conversation dedup (see 0010_messaging.sql for why this
// isn't a DB constraint).
func (r *Repository) FindDirectBetween(ctx context.Context, appID, userA, userB string) (messaging.Conversation, error) {
	var c messaging.Conversation
	err := r.pool.QueryRow(ctx,
		`SELECT c.id, c.app_id, c.type, c.metadata, c.created_at, c.updated_at
		 FROM conversations c
		 WHERE c.app_id = $1 AND c.type = 'direct'
		   AND EXISTS (SELECT 1 FROM conversation_members WHERE conversation_id = c.id AND user_id = $2)
		   AND EXISTS (SELECT 1 FROM conversation_members WHERE conversation_id = c.id AND user_id = $3)
		   AND (SELECT count(*) FROM conversation_members WHERE conversation_id = c.id) = 2
		 LIMIT 1`,
		appID, userA, userB,
	).Scan(&c.ID, &c.AppID, &c.Type, &c.Metadata, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messaging.Conversation{}, messaging.ErrNotFound
		}
		return messaging.Conversation{}, fmt.Errorf("find direct conversation: %w", err)
	}
	return c, nil
}

func (r *Repository) GetConversation(ctx context.Context, id string) (messaging.Conversation, error) {
	var c messaging.Conversation
	err := r.pool.QueryRow(ctx, `SELECT `+conversationColumns+` FROM conversations WHERE id = $1`, id).
		Scan(&c.ID, &c.AppID, &c.Type, &c.Metadata, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messaging.Conversation{}, messaging.ErrNotFound
		}
		return messaging.Conversation{}, fmt.Errorf("get conversation: %w", err)
	}
	return c, nil
}

func (r *Repository) ListConversationsForUser(ctx context.Context, userID string) ([]messaging.Conversation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.app_id, c.type, c.metadata, c.created_at, c.updated_at
		 FROM conversations c JOIN conversation_members m ON m.conversation_id = c.id
		 WHERE m.user_id = $1 ORDER BY c.updated_at DESC`,
		userID)
	if err != nil {
		return nil, fmt.Errorf("list conversations for user: %w", err)
	}
	defer rows.Close()

	var list []messaging.Conversation
	for rows.Next() {
		var c messaging.Conversation
		if err := rows.Scan(&c.ID, &c.AppID, &c.Type, &c.Metadata, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return list, nil
}

func (r *Repository) GetMembership(ctx context.Context, conversationID, userID string) (messaging.ConversationMember, error) {
	var m messaging.ConversationMember
	err := r.pool.QueryRow(ctx,
		`SELECT `+memberColumns+` FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&m.ConversationID, &m.UserID, &m.JoinedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messaging.ConversationMember{}, messaging.ErrMembershipNotFound
		}
		return messaging.ConversationMember{}, fmt.Errorf("get membership: %w", err)
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, conversationID string) ([]messaging.ConversationMember, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+memberColumns+` FROM conversation_members WHERE conversation_id = $1 ORDER BY joined_at`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var list []messaging.ConversationMember
	for rows.Next() {
		var m messaging.ConversationMember
		if err := rows.Scan(&m.ConversationID, &m.UserID, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate members: %w", err)
	}
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, conversationID, userID string) (messaging.ConversationMember, error) {
	var m messaging.ConversationMember
	err := r.pool.QueryRow(ctx,
		`INSERT INTO conversation_members (conversation_id, user_id) VALUES ($1, $2) RETURNING `+memberColumns,
		conversationID, userID,
	).Scan(&m.ConversationID, &m.UserID, &m.JoinedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return messaging.ConversationMember{}, messaging.ErrAlreadyMember
		}
		return messaging.ConversationMember{}, fmt.Errorf("add member: %w", err)
	}
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, conversationID, userID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM conversation_members WHERE conversation_id = $1 AND user_id = $2`, conversationID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return messaging.ErrMembershipNotFound
	}
	return nil
}

// CreateMessage writes the message, its attachments, a Delivery row for
// every other conversation member, and the message.sent outbox event, all
// in one transaction - the durable half of sending a message. Realtime
// push to those same members happens afterward, outside this transaction
// (Service.SendMessage), since a Redis publish has no place in a Postgres
// commit.
func (r *Repository) CreateMessage(ctx context.Context, conversationID, senderID string, in messaging.SendMessageInput) (messaging.Message, error) {
	payload, err := marshalJSON(in.Body)
	if err != nil {
		return messaging.Message{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return messaging.Message{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var m messaging.Message
	err = tx.QueryRow(ctx,
		`INSERT INTO messages (conversation_id, sender_id, message_type, payload) VALUES ($1, $2, $3, $4) RETURNING `+messageColumns,
		conversationID, senderID, in.Type, payload,
	).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &m.CreatedAt)
	if err != nil {
		return messaging.Message{}, fmt.Errorf("insert message: %w", err)
	}

	for _, a := range in.Attachments {
		metadata, err := marshalJSON(a.Metadata)
		if err != nil {
			return messaging.Message{}, err
		}
		var att messaging.Attachment
		err = tx.QueryRow(ctx,
			`INSERT INTO message_attachments (message_id, url, content_type, size_bytes, metadata)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id, message_id, url, content_type, size_bytes, metadata`,
			m.ID, a.URL, a.ContentType, a.SizeBytes, metadata,
		).Scan(&att.ID, &att.MessageID, &att.URL, &att.ContentType, &att.SizeBytes, &att.Metadata)
		if err != nil {
			return messaging.Message{}, fmt.Errorf("insert attachment: %w", err)
		}
		m.Attachments = append(m.Attachments, att)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO message_deliveries (message_id, user_id)
		 SELECT $1, user_id FROM conversation_members WHERE conversation_id = $2 AND user_id <> $3`,
		m.ID, conversationID, senderID,
	); err != nil {
		return messaging.Message{}, fmt.Errorf("insert deliveries: %w", err)
	}

	if err := insertOutboxEvent(ctx, tx, m); err != nil {
		return messaging.Message{}, err
	}

	// A new message is a conversation-level "recent activity" signal, so
	// ListConversationsForUser's most-recently-active-first ordering
	// reflects it.
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conversationID); err != nil {
		return messaging.Message{}, fmt.Errorf("touch conversation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return messaging.Message{}, fmt.Errorf("commit tx: %w", err)
	}
	return m, nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, m messaging.Message) error {
	payload, err := json.Marshal(map[string]any{
		"id": m.ID, "conversationId": m.ConversationID, "senderId": m.SenderID, "type": m.Type,
	})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, event_version, payload, correlation_id)
		 VALUES ('message', $1, 'message.sent', 1, $2, $3)`,
		m.ID, payload, nullIfEmpty(correlation.FromContext(ctx)),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func (r *Repository) GetMessage(ctx context.Context, id string) (messaging.Message, error) {
	var m messaging.Message
	err := r.pool.QueryRow(ctx, `SELECT `+messageColumns+` FROM messages WHERE id = $1`, id).
		Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messaging.Message{}, messaging.ErrMessageNotFound
		}
		return messaging.Message{}, fmt.Errorf("get message: %w", err)
	}
	attachments, err := r.attachmentsFor(ctx, []string{id})
	if err != nil {
		return messaging.Message{}, err
	}
	m.Attachments = attachments[id]
	return m, nil
}

func (r *Repository) ListMessages(ctx context.Context, conversationID string, params messaging.ListParams) (messaging.ListResult, error) {
	var rows pgx.Rows
	var err error
	if params.Cursor == "" {
		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE conversation_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`,
			conversationID, params.Limit+1)
	} else {
		beforeCreated, beforeID, decodeErr := decodeCursor(params.Cursor)
		if decodeErr != nil {
			return messaging.ListResult{}, &messaging.ValidationError{Message: "invalid cursor"}
		}
		rows, err = r.pool.Query(ctx,
			`SELECT `+messageColumns+` FROM messages
			 WHERE conversation_id = $1 AND (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC, id DESC LIMIT $4`,
			conversationID, beforeCreated, beforeID, params.Limit+1)
	}
	if err != nil {
		return messaging.ListResult{}, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var items []messaging.Message
	for rows.Next() {
		var m messaging.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &m.CreatedAt); err != nil {
			return messaging.ListResult{}, fmt.Errorf("scan message: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return messaging.ListResult{}, fmt.Errorf("iterate messages: %w", err)
	}

	result := messaging.ListResult{Items: items}
	if len(items) > params.Limit {
		last := items[params.Limit-1]
		result.Items = items[:params.Limit]
		result.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}

	ids := make([]string, len(result.Items))
	for i, m := range result.Items {
		ids[i] = m.ID
	}
	attachments, err := r.attachmentsFor(ctx, ids)
	if err != nil {
		return messaging.ListResult{}, err
	}
	for i, m := range result.Items {
		result.Items[i].Attachments = attachments[m.ID]
	}
	return result, nil
}

func (r *Repository) attachmentsFor(ctx context.Context, messageIDs []string) (map[string][]messaging.Attachment, error) {
	result := map[string][]messaging.Attachment{}
	if len(messageIDs) == 0 {
		return result, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, message_id, url, content_type, size_bytes, metadata FROM message_attachments WHERE message_id = ANY($1)`,
		messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var a messaging.Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.URL, &a.ContentType, &a.SizeBytes, &a.Metadata); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		result[a.MessageID] = append(result[a.MessageID], a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachments: %w", err)
	}
	return result, nil
}

// MarkDelivered upserts rather than requiring a pre-existing row: a
// sender marking their own message, or any (message, user) pair without a
// Delivery row for whatever reason, self-heals instead of erroring.
// COALESCE keeps the first delivered_at if the client acknowledges twice.
func (r *Repository) MarkDelivered(ctx context.Context, messageID, userID string) (messaging.Delivery, error) {
	var d messaging.Delivery
	err := r.pool.QueryRow(ctx,
		`INSERT INTO message_deliveries (message_id, user_id, delivered_at) VALUES ($1, $2, now())
		 ON CONFLICT (message_id, user_id) DO UPDATE SET delivered_at = COALESCE(message_deliveries.delivered_at, EXCLUDED.delivered_at)
		 RETURNING message_id, user_id, delivered_at`,
		messageID, userID,
	).Scan(&d.MessageID, &d.UserID, &d.DeliveredAt)
	if err != nil {
		return messaging.Delivery{}, fmt.Errorf("mark delivered: %w", err)
	}
	return d, nil
}

// GetReadState returns a zero-value ReadState (no error) when the user
// hasn't read anything in this conversation yet - that's the normal state
// for a brand-new member, not a not-found error.
func (r *Repository) GetReadState(ctx context.Context, conversationID, userID string) (messaging.ReadState, error) {
	var rs messaging.ReadState
	var lastReadMessage *string
	err := r.pool.QueryRow(ctx,
		`SELECT conversation_id, user_id, last_read_message_id, last_read_at FROM conversation_read_states WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&rs.ConversationID, &rs.UserID, &lastReadMessage, &rs.LastReadAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return messaging.ReadState{ConversationID: conversationID, UserID: userID}, nil
		}
		return messaging.ReadState{}, fmt.Errorf("get read state: %w", err)
	}
	if lastReadMessage != nil {
		rs.LastReadMessage = *lastReadMessage
	}
	return rs, nil
}

func (r *Repository) SetReadState(ctx context.Context, conversationID, userID, lastReadMessageID string) (messaging.ReadState, error) {
	var rs messaging.ReadState
	var lastReadMessage *string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO conversation_read_states (conversation_id, user_id, last_read_message_id, last_read_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (conversation_id, user_id) DO UPDATE SET last_read_message_id = $3, last_read_at = now()
		 RETURNING conversation_id, user_id, last_read_message_id, last_read_at`,
		conversationID, userID, lastReadMessageID,
	).Scan(&rs.ConversationID, &rs.UserID, &lastReadMessage, &rs.LastReadAt)
	if err != nil {
		return messaging.ReadState{}, fmt.Errorf("set read state: %w", err)
	}
	if lastReadMessage != nil {
		rs.LastReadMessage = *lastReadMessage
	}
	return rs, nil
}

// AddReaction is an upsert - reacting twice with the same type returns the
// existing row rather than erroring (see Service.AddReaction).
func (r *Repository) AddReaction(ctx context.Context, messageID, userID, reactionType string) (messaging.Reaction, error) {
	var rc messaging.Reaction
	err := r.pool.QueryRow(ctx,
		`INSERT INTO message_reactions (message_id, user_id, reaction_type) VALUES ($1, $2, $3)
		 ON CONFLICT (message_id, user_id, reaction_type) DO UPDATE SET reaction_type = EXCLUDED.reaction_type
		 RETURNING id, message_id, user_id, reaction_type, created_at`,
		messageID, userID, reactionType,
	).Scan(&rc.ID, &rc.MessageID, &rc.UserID, &rc.Type, &rc.CreatedAt)
	if err != nil {
		return messaging.Reaction{}, fmt.Errorf("add reaction: %w", err)
	}
	return rc, nil
}

func (r *Repository) RemoveReaction(ctx context.Context, messageID, userID, reactionType string) error {
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND reaction_type = $3`,
		messageID, userID, reactionType,
	); err != nil {
		return fmt.Errorf("remove reaction: %w", err)
	}
	return nil
}

func (r *Repository) ListReactions(ctx context.Context, messageID string) ([]messaging.Reaction, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, message_id, user_id, reaction_type, created_at FROM message_reactions WHERE message_id = $1 ORDER BY created_at`,
		messageID)
	if err != nil {
		return nil, fmt.Errorf("list reactions: %w", err)
	}
	defer rows.Close()

	var list []messaging.Reaction
	for rows.Next() {
		var rc messaging.Reaction
		if err := rows.Scan(&rc.ID, &rc.MessageID, &rc.UserID, &rc.Type, &rc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan reaction: %w", err)
		}
		list = append(list, rc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reactions: %w", err)
	}
	return list, nil
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
