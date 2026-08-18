// Package memory is an in-memory messaging.Repository for tests. Its
// message-list cursor format is intentionally different from postgres's
// (a bare base64(id) rather than base64(time|id)) - ListParams.Cursor is
// opaque to callers by contract, and this package is never used in
// production, so the two never need to be cross-compatible.
package memory

import (
	"context"
	"encoding/base64"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/core-api/internal/messaging"
)

type Repository struct {
	mu            sync.Mutex
	conversations map[string]messaging.Conversation
	members       map[string]messaging.ConversationMember // conversationID|userID
	messages      map[string]messaging.Message
	deliveries    map[string]messaging.Delivery  // messageID|userID
	readStates    map[string]messaging.ReadState // conversationID|userID
	reactions     map[string]messaging.Reaction  // messageID|userID|type
}

func New() *Repository {
	return &Repository{
		conversations: map[string]messaging.Conversation{},
		members:       map[string]messaging.ConversationMember{},
		messages:      map[string]messaging.Message{},
		deliveries:    map[string]messaging.Delivery{},
		readStates:    map[string]messaging.ReadState{},
		reactions:     map[string]messaging.Reaction{},
	}
}

func memberKey(conversationID, userID string) string    { return conversationID + "|" + userID }
func deliveryKey(messageID, userID string) string       { return messageID + "|" + userID }
func readStateKey(conversationID, userID string) string { return conversationID + "|" + userID }
func reactionKey(messageID, userID, reactionType string) string {
	return messageID + "|" + userID + "|" + reactionType
}

func (r *Repository) CreateConversation(ctx context.Context, creatorUserID string, in messaging.CreateConversationInput) (messaging.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	c := messaging.Conversation{
		ID: uuid.NewString(), AppID: in.AppID, Type: in.Type, Metadata: in.Metadata,
		CreatedAt: now, UpdatedAt: now,
	}
	r.conversations[c.ID] = c

	seen := map[string]bool{}
	for _, userID := range in.MemberUserIDs {
		if seen[userID] {
			continue
		}
		seen[userID] = true
		k := memberKey(c.ID, userID)
		r.members[k] = messaging.ConversationMember{ConversationID: c.ID, UserID: userID, JoinedAt: now}
	}
	return c, nil
}

func (r *Repository) FindDirectBetween(ctx context.Context, appID, userA, userB string) (messaging.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, c := range r.conversations {
		if c.AppID != appID || c.Type != messaging.ConversationDirect {
			continue
		}
		hasA, hasB, count := false, false, 0
		for _, m := range r.members {
			if m.ConversationID != c.ID {
				continue
			}
			count++
			if m.UserID == userA {
				hasA = true
			}
			if m.UserID == userB {
				hasB = true
			}
		}
		if hasA && hasB && count == 2 {
			return c, nil
		}
	}
	return messaging.Conversation{}, messaging.ErrNotFound
}

func (r *Repository) GetConversation(ctx context.Context, id string) (messaging.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.conversations[id]
	if !ok {
		return messaging.Conversation{}, messaging.ErrNotFound
	}
	return c, nil
}

func (r *Repository) ListConversationsForUser(ctx context.Context, userID string) ([]messaging.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []messaging.Conversation
	for _, m := range r.members {
		if m.UserID == userID {
			list = append(list, r.conversations[m.ConversationID])
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	return list, nil
}

func (r *Repository) GetMembership(ctx context.Context, conversationID, userID string) (messaging.ConversationMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[memberKey(conversationID, userID)]
	if !ok {
		return messaging.ConversationMember{}, messaging.ErrMembershipNotFound
	}
	return m, nil
}

func (r *Repository) ListMembers(ctx context.Context, conversationID string) ([]messaging.ConversationMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []messaging.ConversationMember
	for _, m := range r.members {
		if m.ConversationID == conversationID {
			list = append(list, m)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].JoinedAt.Before(list[j].JoinedAt) })
	return list, nil
}

func (r *Repository) AddMember(ctx context.Context, conversationID, userID string) (messaging.ConversationMember, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := memberKey(conversationID, userID)
	if _, exists := r.members[k]; exists {
		return messaging.ConversationMember{}, messaging.ErrAlreadyMember
	}
	m := messaging.ConversationMember{ConversationID: conversationID, UserID: userID, JoinedAt: time.Now().UTC()}
	r.members[k] = m
	return m, nil
}

func (r *Repository) RemoveMember(ctx context.Context, conversationID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := memberKey(conversationID, userID)
	if _, ok := r.members[k]; !ok {
		return messaging.ErrMembershipNotFound
	}
	delete(r.members, k)
	return nil
}

func (r *Repository) CreateMessage(ctx context.Context, conversationID, senderID string, in messaging.SendMessageInput) (messaging.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	m := messaging.Message{
		ID: uuid.NewString(), ConversationID: conversationID, SenderID: senderID,
		Type: in.Type, Body: in.Body, CreatedAt: now,
	}
	for _, a := range in.Attachments {
		m.Attachments = append(m.Attachments, messaging.Attachment{
			ID: uuid.NewString(), MessageID: m.ID, URL: a.URL, ContentType: a.ContentType,
			SizeBytes: a.SizeBytes, Metadata: a.Metadata,
		})
	}
	r.messages[m.ID] = m

	for _, member := range r.members {
		if member.ConversationID == conversationID && member.UserID != senderID {
			r.deliveries[deliveryKey(m.ID, member.UserID)] = messaging.Delivery{MessageID: m.ID, UserID: member.UserID}
		}
	}

	if c, ok := r.conversations[conversationID]; ok {
		c.UpdatedAt = now
		r.conversations[conversationID] = c
	}
	return m, nil
}

func (r *Repository) GetMessage(ctx context.Context, id string) (messaging.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.messages[id]
	if !ok {
		return messaging.Message{}, messaging.ErrMessageNotFound
	}
	return m, nil
}

func (r *Repository) ListMessages(ctx context.Context, conversationID string, params messaging.ListParams) (messaging.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []messaging.Message
	for _, m := range r.messages {
		if m.ConversationID == conversationID {
			all = append(all, m)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	start := 0
	if params.Cursor != "" {
		afterID, err := decodeCursor(params.Cursor)
		if err != nil {
			return messaging.ListResult{}, &messaging.ValidationError{Message: "invalid cursor"}
		}
		for i, m := range all {
			if m.ID == afterID {
				start = i + 1
				break
			}
		}
	}

	end := start + params.Limit
	if end > len(all) {
		end = len(all)
	}
	if start > len(all) {
		start = len(all)
	}
	page := append([]messaging.Message{}, all[start:end]...)

	result := messaging.ListResult{Items: page}
	if end < len(all) {
		result.NextCursor = encodeCursor(page[len(page)-1].ID)
	}
	return result, nil
}

func encodeCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func decodeCursor(s string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	return string(raw), err
}

func (r *Repository) MarkDelivered(ctx context.Context, messageID, userID string) (messaging.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := deliveryKey(messageID, userID)
	d, ok := r.deliveries[k]
	if !ok {
		d = messaging.Delivery{MessageID: messageID, UserID: userID}
	}
	if d.DeliveredAt == nil {
		now := time.Now().UTC()
		d.DeliveredAt = &now
	}
	r.deliveries[k] = d
	return d, nil
}

func (r *Repository) GetReadState(ctx context.Context, conversationID, userID string) (messaging.ReadState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rs, ok := r.readStates[readStateKey(conversationID, userID)]
	if !ok {
		return messaging.ReadState{ConversationID: conversationID, UserID: userID}, nil
	}
	return rs, nil
}

func (r *Repository) SetReadState(ctx context.Context, conversationID, userID, lastReadMessageID string) (messaging.ReadState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	rs := messaging.ReadState{ConversationID: conversationID, UserID: userID, LastReadMessage: lastReadMessageID, LastReadAt: &now}
	r.readStates[readStateKey(conversationID, userID)] = rs
	return rs, nil
}

func (r *Repository) AddReaction(ctx context.Context, messageID, userID, reactionType string) (messaging.Reaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := reactionKey(messageID, userID, reactionType)
	if existing, ok := r.reactions[k]; ok {
		return existing, nil
	}
	rc := messaging.Reaction{ID: uuid.NewString(), MessageID: messageID, UserID: userID, Type: reactionType, CreatedAt: time.Now().UTC()}
	r.reactions[k] = rc
	return rc, nil
}

func (r *Repository) RemoveReaction(ctx context.Context, messageID, userID, reactionType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.reactions, reactionKey(messageID, userID, reactionType))
	return nil
}

func (r *Repository) ListReactions(ctx context.Context, messageID string) ([]messaging.Reaction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []messaging.Reaction
	for _, rc := range r.reactions {
		if rc.MessageID == messageID {
			list = append(list, rc)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list, nil
}
