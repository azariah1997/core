package messaging_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/messaging"
	"github.com/example/core-platform/backend/core-api/internal/messaging/memory"
)

// fakeRealtime records every push so tests can assert who was (and wasn't)
// notified, without needing a real Redis - the same reason Realtime is a
// narrow interface Service depends on rather than a concrete rtbus type.
type fakeRealtime struct {
	mu    sync.Mutex
	calls []realtimeCall
}

type realtimeCall struct {
	UserID  string
	Payload json.RawMessage
}

func (f *fakeRealtime) ToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, realtimeCall{UserID: userID, Payload: payload})
	return nil
}

func (f *fakeRealtime) notifiedUsers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, c := range f.calls {
		ids = append(ids, c.UserID)
	}
	return ids
}

func newService() (*messaging.Service, *fakeRealtime) {
	rt := &fakeRealtime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return messaging.NewService(memory.New(), rt, logger), rt
}

func TestCreateDirectConversationRejectsWrongMemberCount(t *testing.T) {
	svc, _ := newService()
	_, err := svc.CreateConversation(context.Background(), "u1",
		messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationDirect, MemberUserIDs: []string{"u1", "u2", "u3"}})
	var verr *messaging.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestCreateDirectConversationIsIdempotent(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	in := messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationDirect, MemberUserIDs: []string{"u1", "u2"}}

	first, err := svc.CreateConversation(ctx, "u1", in)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.CreateConversation(ctx, "u1", in)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected the same direct conversation to be reused, got %s and %s", first.ID, second.ID)
	}
}

func TestNonMemberCannotGetConversation(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(ctx, "stranger", c.ID); !errors.Is(err, messaging.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestGroupConversationAllowsAddingMembers(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "u1", c.ID, "u2"); err != nil {
		t.Fatalf("add member: %v", err)
	}
	members, err := svc.ListMembers(ctx, "u1", c.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

func TestDirectConversationRejectsAddMember(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationDirect, MemberUserIDs: []string{"u1", "u2"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.AddMember(ctx, "u1", c.ID, "u3"); !errors.Is(err, messaging.ErrMembershipFixed) {
		t.Fatalf("expected ErrMembershipFixed, got %v", err)
	}
}

func TestOnlySelfCanLeaveConversation(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1", "u2"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.RemoveMember(ctx, "u1", c.ID, "u2"); !errors.Is(err, messaging.ErrCannotRemoveSelf) {
		t.Fatalf("expected ErrCannotRemoveSelf, got %v", err)
	}
	if err := svc.RemoveMember(ctx, "u2", c.ID, "u2"); err != nil {
		t.Fatalf("self leave: %v", err)
	}
}

func TestSendMessageRequiresMembership(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.SendMessage(ctx, "stranger", c.ID, messaging.SendMessageInput{Type: "TEXT", Body: map[string]any{"text": "hi"}})
	if !errors.Is(err, messaging.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestFreeFormMessageTypeIsNeverValidated(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// MessageType is a free-form, product-defined string - a made-up type
	// like "STICKER" is just as valid as the roadmap's own TEXT/IMAGE/FILE
	// examples; the platform never enforces a fixed type enum.
	msg, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "STICKER", Body: map[string]any{"ref": "party-parrot"}})
	if err != nil {
		t.Fatalf("send message with arbitrary type: %v", err)
	}
	if msg.Type != "STICKER" {
		t.Fatalf("expected type 'STICKER' to be preserved as-is, got %q", msg.Type)
	}
}

func TestSendMessageNotifiesOtherMembersNotSender(t *testing.T) {
	svc, rt := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1", "u2", "u3"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "TEXT", Body: map[string]any{"text": "hi"}}); err != nil {
		t.Fatalf("send: %v", err)
	}

	notified := rt.notifiedUsers()
	if len(notified) != 2 {
		t.Fatalf("expected exactly 2 notifications (u2, u3), got %v", notified)
	}
	for _, id := range notified {
		if id == "u1" {
			t.Fatalf("sender should not be notified of their own message, got %v", notified)
		}
	}
}

func TestSendMessageCreatesDeliveriesForOtherMembers(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1", "u2"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	msg, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "TEXT"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	d, err := svc.MarkDelivered(ctx, "u2", msg.ID)
	if err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if d.DeliveredAt == nil {
		t.Fatal("expected delivery to be recorded")
	}
	// Idempotent: acking twice keeps the first timestamp rather than erroring.
	d2, err := svc.MarkDelivered(ctx, "u2", msg.ID)
	if err != nil {
		t.Fatalf("mark delivered again: %v", err)
	}
	if !d2.DeliveredAt.Equal(*d.DeliveredAt) {
		t.Fatalf("expected delivered_at to stay stable across repeated acks, got %v then %v", d.DeliveredAt, d2.DeliveredAt)
	}
}

func TestListMessagesPagination(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "TEXT"}); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	page1, err := svc.ListMessages(ctx, "u1", c.ID, messaging.ListParams{Limit: 2})
	if err != nil {
		t.Fatalf("list page1: %v", err)
	}
	if len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("expected a 2-item page with a next cursor, got %d items, cursor %q", len(page1.Items), page1.NextCursor)
	}

	seen := map[string]bool{page1.Items[0].ID: true, page1.Items[1].ID: true}
	cursor := page1.NextCursor
	for {
		page, err := svc.ListMessages(ctx, "u1", c.ID, messaging.ListParams{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("list next page: %v", err)
		}
		for _, m := range page.Items {
			if seen[m.ID] {
				t.Fatalf("message %s appeared twice across pages", m.ID)
			}
			seen[m.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("expected to see all 5 messages across pages, got %d", len(seen))
	}
}

func TestReadStateRoundTrip(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	msg, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "TEXT"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	// Brand new member: no read state yet is a normal zero-value, not an error.
	rs, err := svc.GetReadState(ctx, "u1", c.ID, "u1")
	if err != nil {
		t.Fatalf("get read state before any read: %v", err)
	}
	if rs.LastReadMessage != "" {
		t.Fatalf("expected empty read state, got %+v", rs)
	}

	updated, err := svc.SetReadState(ctx, "u1", c.ID, msg.ID)
	if err != nil {
		t.Fatalf("set read state: %v", err)
	}
	if updated.LastReadMessage != msg.ID || updated.LastReadAt == nil {
		t.Fatalf("unexpected read state after set: %+v", updated)
	}
}

func TestReactionsAreIdempotent(t *testing.T) {
	svc, _ := newService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, "u1", messaging.CreateConversationInput{AppID: "app-1", Type: messaging.ConversationGroup, MemberUserIDs: []string{"u1", "u2"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	msg, err := svc.SendMessage(ctx, "u1", c.ID, messaging.SendMessageInput{Type: "TEXT"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	first, err := svc.AddReaction(ctx, "u2", msg.ID, "👍")
	if err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	second, err := svc.AddReaction(ctx, "u2", msg.ID, "👍")
	if err != nil {
		t.Fatalf("add reaction again: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected reacting twice to be a no-op returning the same row, got %s and %s", first.ID, second.ID)
	}

	if err := svc.RemoveReaction(ctx, "u2", msg.ID, "👍"); err != nil {
		t.Fatalf("remove reaction: %v", err)
	}
	// Removing an already-removed reaction is not an error - a common
	// double-tap client race, not a genuine conflict.
	if err := svc.RemoveReaction(ctx, "u2", msg.ID, "👍"); err != nil {
		t.Fatalf("remove reaction again: %v", err)
	}
	reactions, err := svc.ListReactions(ctx, "u1", msg.ID)
	if err != nil {
		t.Fatalf("list reactions: %v", err)
	}
	if len(reactions) != 0 {
		t.Fatalf("expected no reactions left, got %+v", reactions)
	}
}
