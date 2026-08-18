// Package messaging builds durable conversations and messages on top of
// generic platform capabilities: Postgres for storage, the transactional
// outbox for the message.sent domain event, and the Realtime Gateway
// (Phase 10) for pushing new messages to connected clients. Realtime
// delivery and durable storage are deliberately separate concerns here - a
// message is durably written first, and push notification to connected
// members happens afterward, best-effort, over the same Redis pub/sub bus
// realtime-gateway's hub already listens on.
package messaging

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ConversationType is a closed set - unlike MessageType below, the roadmap
// names exactly these three kinds of conversation, so it's validated
// against a fixed enum rather than left as a free-form product label.
type ConversationType string

const (
	ConversationDirect ConversationType = "direct"
	ConversationGroup  ConversationType = "group"
	ConversationCustom ConversationType = "custom"
)

func (t ConversationType) valid() bool {
	switch t {
	case ConversationDirect, ConversationGroup, ConversationCustom:
		return true
	default:
		return false
	}
}

type Conversation struct {
	ID        string
	AppID     string
	Type      ConversationType
	Metadata  map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConversationMember has no ID of its own - conversation_members is a pure
// join table (PRIMARY KEY(conversation_id, user_id) in the pre-existing
// scaffold schema this package builds on), so the pair already identifies
// a membership uniquely; nothing here ever looks one up by anything else.
type ConversationMember struct {
	ConversationID string
	UserID         string
	JoinedAt       time.Time
}

// Message.Type is deliberately a free-form, product-defined string, never
// validated against a fixed enum - TEXT/IMAGE/FILE/SYSTEM/CUSTOM are the
// roadmap's own examples of a type future products can extend, the same
// "do not hardcode product concepts" convention this repo already applies
// to RelationshipType and GroupMember.Role.
type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	Type           string
	Body           map[string]any
	Attachments    []Attachment
	CreatedAt      time.Time
}

type Attachment struct {
	ID          string
	MessageID   string
	URL         string
	ContentType string
	SizeBytes   int64
	Metadata    map[string]any
}

// Delivery tracks, per recipient, whether a message has reached their
// client. It's populated for every other conversation member at send time
// and updated via an explicit client acknowledgement - this phase doesn't
// add a new WebSocket message type for it (Phase 10's protocol is generic
// transport, not domain-specific acks), so the ack travels over the same
// authenticated REST API as everything else.
type Delivery struct {
	MessageID   string
	UserID      string
	DeliveredAt *time.Time
}

// ReadState is deliberately its own record rather than a field on
// ConversationMember - "who's in the conversation" and "how far have they
// read" are different questions with different write patterns (the former
// changes rarely, the latter potentially on every message view).
type ReadState struct {
	ConversationID  string
	UserID          string
	LastReadMessage string
	LastReadAt      *time.Time
}

// Reaction.Type is free-form for the same reason Message.Type is - an
// emoji or a product-defined reaction name, never a fixed platform enum.
type Reaction struct {
	ID        string
	MessageID string
	UserID    string
	Type      string
	CreatedAt time.Time
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

var (
	ErrNotFound           = errors.New("conversation not found")
	ErrMessageNotFound    = errors.New("message not found")
	ErrForbidden          = errors.New("not a member of this conversation")
	ErrAlreadyMember      = errors.New("user is already a member")
	ErrMembershipNotFound = errors.New("membership not found")
	ErrMembershipFixed    = errors.New("direct conversation membership cannot be changed")
	ErrCannotRemoveSelf   = errors.New("cannot remove another member; members may only leave themselves")
)

type CreateConversationInput struct {
	AppID         string
	Type          ConversationType
	MemberUserIDs []string
	Metadata      map[string]any
}

func (in CreateConversationInput) Validate() error {
	switch {
	case strings.TrimSpace(in.AppID) == "":
		return &ValidationError{"appId is required"}
	case !in.Type.valid():
		return &ValidationError{"type must be one of direct, group, custom"}
	}
	seen := map[string]bool{}
	unique := 0
	for _, id := range in.MemberUserIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return &ValidationError{"memberUserIds cannot contain an empty id"}
		}
		if !seen[id] {
			seen[id] = true
			unique++
		}
	}
	if unique == 0 {
		return &ValidationError{"at least one member is required"}
	}
	if in.Type == ConversationDirect && unique != 2 {
		return &ValidationError{"a direct conversation requires exactly two distinct members"}
	}
	return nil
}

type SendMessageInput struct {
	Type        string
	Body        map[string]any
	Attachments []AttachmentInput
}

type AttachmentInput struct {
	URL         string
	ContentType string
	SizeBytes   int64
	Metadata    map[string]any
}

func (in SendMessageInput) Validate() error {
	if strings.TrimSpace(in.Type) == "" {
		return &ValidationError{"type is required"}
	}
	for _, a := range in.Attachments {
		if strings.TrimSpace(a.URL) == "" {
			return &ValidationError{"attachment url is required"}
		}
	}
	return nil
}

// ListParams mirror applications.ListParams: an opaque cursor from a
// previous ListResult, never a raw offset, so pages stay stable as new
// messages arrive concurrently.
type ListParams struct {
	Limit  int
	Cursor string
}

type ListResult struct {
	Items      []Message
	NextCursor string
}

// Repository is the storage-agnostic boundary. Implementations own
// emitting the message.sent outbox event atomically with the message
// write, the same pattern as every other domain module's Repository.
type Repository interface {
	CreateConversation(ctx context.Context, creatorUserID string, in CreateConversationInput) (Conversation, error)
	FindDirectBetween(ctx context.Context, appID, userA, userB string) (Conversation, error)
	GetConversation(ctx context.Context, id string) (Conversation, error)
	ListConversationsForUser(ctx context.Context, userID string) ([]Conversation, error)

	GetMembership(ctx context.Context, conversationID, userID string) (ConversationMember, error)
	ListMembers(ctx context.Context, conversationID string) ([]ConversationMember, error)
	AddMember(ctx context.Context, conversationID, userID string) (ConversationMember, error)
	RemoveMember(ctx context.Context, conversationID, userID string) error

	CreateMessage(ctx context.Context, conversationID, senderID string, in SendMessageInput) (Message, error)
	GetMessage(ctx context.Context, id string) (Message, error)
	ListMessages(ctx context.Context, conversationID string, params ListParams) (ListResult, error)

	MarkDelivered(ctx context.Context, messageID, userID string) (Delivery, error)
	GetReadState(ctx context.Context, conversationID, userID string) (ReadState, error)
	SetReadState(ctx context.Context, conversationID, userID, lastReadMessageID string) (ReadState, error)

	AddReaction(ctx context.Context, messageID, userID, reactionType string) (Reaction, error)
	RemoveReaction(ctx context.Context, messageID, userID, reactionType string) error
	ListReactions(ctx context.Context, messageID string) ([]Reaction, error)
}
