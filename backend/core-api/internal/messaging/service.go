package messaging

import (
	"context"
	"encoding/json"
	"log/slog"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// Realtime is the narrow publish surface Service needs to push a new
// message to connected clients - satisfied directly by
// platformkit/rtbus.Publisher in production, and a fake in tests, the same
// consumer-defined-interface pattern used elsewhere in this repo to keep a
// domain module decoupled from another module's or package's concrete
// type.
type Realtime interface {
	ToUser(ctx context.Context, userID string, payload json.RawMessage) error
}

type Service struct {
	repo   Repository
	rt     Realtime
	logger *slog.Logger
}

func NewService(repo Repository, rt Realtime, logger *slog.Logger) *Service {
	return &Service{repo: repo, rt: rt, logger: logger}
}

// CreateConversation needs no membership check - anyone authenticated may
// start one, becoming its first member. For "direct" conversations it's
// idempotent: requesting a new direct conversation between the same pair
// of users returns the existing one rather than creating a duplicate
// thread, matching what every real chat product's "message this person"
// action expects.
func (s *Service) CreateConversation(ctx context.Context, creatorUserID string, in CreateConversationInput) (Conversation, error) {
	if err := in.Validate(); err != nil {
		return Conversation{}, err
	}
	if in.Type == ConversationDirect {
		other := otherMember(in.MemberUserIDs, creatorUserID)
		if existing, err := s.repo.FindDirectBetween(ctx, in.AppID, creatorUserID, other); err == nil {
			return existing, nil
		} else if err != ErrNotFound {
			return Conversation{}, err
		}
	}
	return s.repo.CreateConversation(ctx, creatorUserID, in)
}

func otherMember(memberUserIDs []string, creatorUserID string) string {
	for _, id := range memberUserIDs {
		if id != creatorUserID {
			return id
		}
	}
	return creatorUserID
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Conversation, error) {
	if _, err := s.requireMember(ctx, id, callerID); err != nil {
		return Conversation{}, err
	}
	return s.repo.GetConversation(ctx, id)
}

func (s *Service) ListMine(ctx context.Context, userID string) ([]Conversation, error) {
	return s.repo.ListConversationsForUser(ctx, userID)
}

func (s *Service) ListMembers(ctx context.Context, callerID, conversationID string) ([]ConversationMember, error) {
	if _, err := s.requireMember(ctx, conversationID, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, conversationID)
}

// AddMember requires the caller to already be a member; direct
// conversations reject it outright - a direct thread's two participants
// are fixed at creation, matching ConversationType's closed semantics.
func (s *Service) AddMember(ctx context.Context, callerID, conversationID, userID string) (ConversationMember, error) {
	conv, err := s.requireMember(ctx, conversationID, callerID)
	if err != nil {
		return ConversationMember{}, err
	}
	if conv.Type == ConversationDirect {
		return ConversationMember{}, ErrMembershipFixed
	}
	return s.repo.AddMember(ctx, conversationID, userID)
}

// RemoveMember only supports self-removal ("leave"). Unlike groups.Service,
// there's no roadmap-defined role/permission concept for conversations
// that would justify letting one member remove another - that's deferred
// to whichever future need actually requires it.
func (s *Service) RemoveMember(ctx context.Context, callerID, conversationID, targetUserID string) error {
	if callerID != targetUserID {
		return ErrCannotRemoveSelf
	}
	if _, err := s.requireMember(ctx, conversationID, callerID); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, conversationID, targetUserID)
}

// SendMessage durably writes the message first (including per-recipient
// Delivery rows and the message.sent outbox event, all in the repository's
// transaction), then - only once that's committed - best-effort notifies
// every other member in real time. A realtime publish failure never fails
// the send: the message already exists and the recipient will see it next
// time they list the conversation regardless.
func (s *Service) SendMessage(ctx context.Context, callerID, conversationID string, in SendMessageInput) (Message, error) {
	conv, err := s.requireMember(ctx, conversationID, callerID)
	if err != nil {
		return Message{}, err
	}
	if err := in.Validate(); err != nil {
		return Message{}, err
	}

	msg, err := s.repo.CreateMessage(ctx, conversationID, callerID, in)
	if err != nil {
		return Message{}, err
	}

	s.notifyMembers(ctx, conv, msg, callerID)
	return msg, nil
}

func (s *Service) notifyMembers(ctx context.Context, conv Conversation, msg Message, senderID string) {
	members, err := s.repo.ListMembers(ctx, conv.ID)
	if err != nil {
		s.logger.Error("messaging: failed to list members for realtime notify", "error", err, "conversationId", conv.ID)
		return
	}
	payload, err := json.Marshal(realtimePushEvent{
		Type:           "message.new",
		ConversationID: conv.ID,
		Message:        toRealtimeMessage(msg),
	})
	if err != nil {
		s.logger.Error("messaging: failed to marshal realtime payload", "error", err, "conversationId", conv.ID)
		return
	}
	for _, m := range members {
		if m.UserID == senderID {
			continue
		}
		if err := s.rt.ToUser(ctx, m.UserID, payload); err != nil {
			s.logger.Error("messaging: realtime notify failed", "error", err, "conversationId", conv.ID, "userId", m.UserID)
		}
	}
}

type realtimePushEvent struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversationId"`
	Message        realtimeMessage `json:"message"`
}

type realtimeMessage struct {
	ID        string         `json:"id"`
	SenderID  string         `json:"senderId"`
	Type      string         `json:"type"`
	Body      map[string]any `json:"body,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

func toRealtimeMessage(m Message) realtimeMessage {
	return realtimeMessage{
		ID: m.ID, SenderID: m.SenderID, Type: m.Type, Body: m.Body,
		CreatedAt: m.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

func (s *Service) GetMessage(ctx context.Context, callerID, id string) (Message, error) {
	msg, err := s.repo.GetMessage(ctx, id)
	if err != nil {
		return Message{}, err
	}
	if _, err := s.requireMember(ctx, msg.ConversationID, callerID); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (s *Service) ListMessages(ctx context.Context, callerID, conversationID string, params ListParams) (ListResult, error) {
	if _, err := s.requireMember(ctx, conversationID, callerID); err != nil {
		return ListResult{}, err
	}
	if params.Limit <= 0 || params.Limit > maxListLimit {
		params.Limit = defaultListLimit
	}
	return s.repo.ListMessages(ctx, conversationID, params)
}

// MarkDelivered records that callerID's client has received messageID.
// The caller must be the intended recipient, not just any conversation
// member, matching a Delivery row's meaning: has *this specific user's*
// client actually gotten it.
func (s *Service) MarkDelivered(ctx context.Context, callerID, messageID string) (Delivery, error) {
	return s.repo.MarkDelivered(ctx, messageID, callerID)
}

func (s *Service) SetReadState(ctx context.Context, callerID, conversationID, lastReadMessageID string) (ReadState, error) {
	if _, err := s.requireMember(ctx, conversationID, callerID); err != nil {
		return ReadState{}, err
	}
	return s.repo.SetReadState(ctx, conversationID, callerID, lastReadMessageID)
}

func (s *Service) GetReadState(ctx context.Context, callerID, conversationID, targetUserID string) (ReadState, error) {
	if _, err := s.requireMember(ctx, conversationID, callerID); err != nil {
		return ReadState{}, err
	}
	return s.repo.GetReadState(ctx, conversationID, targetUserID)
}

// AddReaction is idempotent - reacting twice with the same type is a no-op
// returning the existing reaction, not a conflict, matching how reactions
// behave as a toggleable client affordance rather than a strict resource
// create.
func (s *Service) AddReaction(ctx context.Context, callerID, messageID, reactionType string) (Reaction, error) {
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		return Reaction{}, err
	}
	if _, err := s.requireMember(ctx, msg.ConversationID, callerID); err != nil {
		return Reaction{}, err
	}
	return s.repo.AddReaction(ctx, messageID, callerID, reactionType)
}

// RemoveReaction is likewise idempotent: removing a reaction that's
// already gone (a common double-tap client race) is not an error.
func (s *Service) RemoveReaction(ctx context.Context, callerID, messageID, reactionType string) error {
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if _, err := s.requireMember(ctx, msg.ConversationID, callerID); err != nil {
		return err
	}
	return s.repo.RemoveReaction(ctx, messageID, callerID, reactionType)
}

func (s *Service) ListReactions(ctx context.Context, callerID, messageID string) ([]Reaction, error) {
	msg, err := s.repo.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireMember(ctx, msg.ConversationID, callerID); err != nil {
		return nil, err
	}
	return s.repo.ListReactions(ctx, messageID)
}

func (s *Service) requireMember(ctx context.Context, conversationID, userID string) (Conversation, error) {
	if _, err := s.repo.GetMembership(ctx, conversationID, userID); err != nil {
		return Conversation{}, ErrForbidden
	}
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return Conversation{}, err
	}
	return conv, nil
}
