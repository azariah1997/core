package livetouch

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// inviteRateLimit/-Window: Live Touch invites are a Bond-only,
// deliberate action, not a rapid-fire gesture like Pulse - a much lower
// ceiling than pulse-interactions' 10/minute is appropriate.
const (
	inviteRateLimit  = 10
	inviteRateWindow = time.Hour
)

type Service struct {
	repo      Repository
	bond      Bond
	realtime  Realtime
	analytics Analytics
	limiter   RateLimiter
}

// bond, realtime, analytics, and limiter are all fixed, service-level
// dependencies (bond reads only Pulse's own Postgres; realtime and
// limiter are shared Redis clients; analytics needs no caller token) -
// only Presence and Notifier below need a per-caller factory, since
// both authenticate to another real service as the caller.
func NewService(repo Repository, bond Bond, realtime Realtime, analytics Analytics, limiter RateLimiter) *Service {
	return &Service{repo: repo, bond: bond, realtime: realtime, analytics: analytics, limiter: limiter}
}

// Invite creates a Live Touch session with the caller's real, current
// active Bond partner (spec §7: Bond "unlocks... Live Touch" - never
// merely a Friend). presence decides the invite's DeliveryMode exactly
// once, the same "decided once, from a real check" discipline
// pulse-interactions' own Start already established; notifier is the
// durable fallback when the receiver isn't connected to realtime-gateway
// right now.
func (s *Service) Invite(ctx context.Context, presence Presence, notifier Notifier, callerID string) (Session, error) {
	b, err := s.bond.MyActiveBond(ctx, callerID)
	if err != nil {
		if errors.Is(err, ErrNoBond) {
			return Session{}, ErrNotBonded
		}
		return Session{}, err
	}
	receiverID := b.otherUser(callerID)

	allowed, err := s.limiter.Allow(ctx, "livetouch:invite:"+callerID, inviteRateLimit, inviteRateWindow)
	if err != nil {
		return Session{}, err
	}
	if !allowed {
		return Session{}, ErrRateLimited
	}

	deliveryMode := DeliveryPush
	if online, err := presence.IsOnline(ctx, receiverID); err == nil && online {
		deliveryMode = DeliveryLive
	}

	now := time.Now().UTC()
	created, err := s.repo.Create(ctx, Session{
		ID: uuid.NewString(), InitiatorID: callerID, ReceiverID: receiverID,
		Status: StatusInvited, DeliveryMode: deliveryMode, InvitedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Session{}, err
	}

	switch deliveryMode {
	case DeliveryLive:
		s.publishLifecycle(ctx, "live_touch.invited", created.ReceiverID, map[string]any{
			"sessionId": created.ID, "initiatorId": created.InitiatorID, "channel": created.Channel(),
		})
	case DeliveryPush:
		_ = notifier.Notify(ctx, created.ReceiverID, "live_touch_invited", "Live Touch",
			"wants to Live Touch with you", map[string]any{"sessionId": created.ID})
	}
	_ = s.analytics.Track(ctx, "live_touch_invited", created.InitiatorID, map[string]any{
		"sessionId": created.ID, "receiverId": created.ReceiverID, "deliveryMode": string(deliveryMode),
	})
	return created, nil
}

// Accept transitions invited -> active. Both participants may now
// subscribe to the session's own realtime channel and exchange touch
// events directly (see this package's header comment).
func (s *Service) Accept(ctx context.Context, callerID, id string) (Session, error) {
	session, err := s.getFresh(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if session.ReceiverID != callerID {
		return Session{}, ErrForbidden
	}
	if session.Status != StatusInvited {
		return Session{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	accepted, err := s.repo.Accept(ctx, id, now)
	if err != nil {
		return Session{}, err
	}

	s.publishLifecycle(ctx, "live_touch.accepted", accepted.InitiatorID, map[string]any{
		"sessionId": accepted.ID, "channel": accepted.Channel(),
	})
	latencyMs := 0
	if accepted.AcceptedAt != nil {
		latencyMs = int(accepted.AcceptedAt.Sub(accepted.InvitedAt).Milliseconds())
	}
	_ = s.analytics.Track(ctx, "live_touch_accepted", accepted.ReceiverID, map[string]any{
		"sessionId": accepted.ID, "inviteToAcceptLatencyMs": latencyMs,
	})
	return accepted, nil
}

// Decline transitions invited -> ended (declined). Only the receiver -
// the one being invited - may decline; the initiator withdraws an
// invite via End instead, matching spec's own separate "session
// invitation" vs. "session completion" DoD items.
func (s *Service) Decline(ctx context.Context, callerID, id string) (Session, error) {
	session, err := s.getFresh(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if session.ReceiverID != callerID {
		return Session{}, ErrForbidden
	}
	if session.Status != StatusInvited {
		return Session{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	ended, err := s.repo.End(ctx, id, now, EndReasonDeclined)
	if err != nil {
		return Session{}, err
	}
	s.publishLifecycle(ctx, "live_touch.declined", ended.InitiatorID, map[string]any{"sessionId": ended.ID})
	_ = s.analytics.Track(ctx, "live_touch_declined", ended.ReceiverID, map[string]any{"sessionId": ended.ID})
	return ended, nil
}

// End is available to either participant, from either invited (the
// initiator cancelling, or the receiver backing out without a formal
// decline) or active (a normal session completion) - the one
// requirement is that it hasn't already ended. Duration is computed
// server-side from the row's own AcceptedAt/EndedAt, never trusted from
// the client (same "server timestamps are truth" discipline as every
// other interaction in this codebase) - zero for a session that was
// never accepted.
func (s *Service) End(ctx context.Context, callerID, id string) (Session, error) {
	session, err := s.getFresh(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if !session.isParticipant(callerID) {
		return Session{}, ErrForbidden
	}
	if session.Status == StatusEnded {
		return Session{}, ErrInvalidTransition
	}

	now := time.Now().UTC()
	ended, err := s.repo.End(ctx, id, now, EndReasonEnded)
	if err != nil {
		return Session{}, err
	}

	s.publishLifecycle(ctx, "live_touch.ended", ended.otherUser(callerID), map[string]any{"sessionId": ended.ID})
	durationMs := 0
	if ended.DurationMs != nil {
		durationMs = *ended.DurationMs
	}
	_ = s.analytics.Track(ctx, "live_touch_completed", callerID, map[string]any{
		"sessionId": ended.ID, "durationMs": durationMs, "endReason": string(EndReasonEnded),
	})
	return ended, nil
}

// Get returns the session as seen by callerID - either participant may
// read it, no one else.
func (s *Service) Get(ctx context.Context, callerID, id string) (Session, error) {
	session, err := s.getFresh(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if !session.isParticipant(callerID) {
		return Session{}, ErrForbidden
	}
	return session, nil
}

// getFresh fetches the session and lazily expires it if it's overdue -
// an unaccepted invite past inviteTimeout, or an active session with no
// End call past abandonedTimeout (see EndReasonTimeout's own doc
// comment) - computed on read, never a background sweep job, the same
// pattern mood's own expiry already established.
func (s *Service) getFresh(ctx context.Context, id string) (Session, error) {
	session, err := s.repo.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	switch {
	case session.Status == StatusInvited && now.Sub(session.InvitedAt) > inviteTimeout:
		return s.repo.End(ctx, id, now, EndReasonTimeout)
	case session.Status == StatusActive && session.AcceptedAt != nil && now.Sub(*session.AcceptedAt) > abandonedTimeout:
		return s.repo.End(ctx, id, now, EndReasonTimeout)
	default:
		return session, nil
	}
}

type lifecyclePayload struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// publishLifecycle is best-effort, same discipline as pulse-interactions'
// own publishPush - a failed live announcement never fails the
// underlying state transition, since the durable session record is the
// source of truth.
func (s *Service) publishLifecycle(ctx context.Context, eventType, userID string, data map[string]any) {
	payload, err := json.Marshal(lifecyclePayload{Type: eventType, Data: data})
	if err != nil {
		return
	}
	_ = s.realtime.ToUser(ctx, userID, payload)
}
