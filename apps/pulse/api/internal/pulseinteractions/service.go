package pulseinteractions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// pulseSendRateLimit/-Window bound how often one sender can start a
// Pulse - spec §37's "never hardcode one global limit" is honored at
// the call-site level (the key is per-sender), even though the actual
// numbers are a code constant this phase, not yet remote-configurable
// (that lands with the rest of Pulse's remote-config surface in
// Phase 13, matching the roadmap's own incremental scope).
const (
	pulseSendRateLimit  = 10
	pulseSendRateWindow = time.Minute
)

type Service struct {
	repo      Repository
	realtime  Realtime
	analytics Analytics
	limiter   RateLimiter
}

func NewService(repo Repository, realtime Realtime, analytics Analytics, limiter RateLimiter) *Service {
	return &Service{repo: repo, realtime: realtime, analytics: analytics, limiter: limiter}
}

// checkConnected implements the authorization model's connection check
// (apps/pulse/docs/ARCHITECTURE_AUDIT.md): Pulse is allowed between an
// active Friend or an active Bond - never a stranger, never a blocked
// relationship, in either direction. core.ListMine is already scoped to
// the current authenticated caller (see pulseauth), so no callerID
// parameter is needed here - only who the interaction targets.
func checkConnected(ctx context.Context, core CoreRelationships, otherUserID string) error {
	for _, relType := range []string{FriendRelationshipType, BondRelationshipType} {
		rels, err := core.ListMine(ctx, relType)
		if err != nil {
			return err
		}
		for _, r := range rels {
			if r.RequesterID != otherUserID && r.TargetID != otherUserID {
				continue
			}
			switch r.Status {
			case "active":
				return nil
			case "blocked":
				return ErrBlocked
			}
		}
	}
	return ErrNotConnected
}

func (s *Service) Create(ctx context.Context, core CoreRelationships, callerID string, in CreateInput) (Interaction, error) {
	if err := in.Validate(); err != nil {
		return Interaction{}, err
	}
	if err := checkConnected(ctx, core, in.ReceiverID); err != nil {
		return Interaction{}, err
	}
	allowed, err := s.limiter.Allow(ctx, "pulseinteractions:send:"+callerID, pulseSendRateLimit, pulseSendRateWindow)
	if err != nil {
		return Interaction{}, err
	}
	if !allowed {
		return Interaction{}, ErrRateLimited
	}
	now := time.Now().UTC()
	i := Interaction{
		ID: uuid.NewString(), Type: TypePulse, SenderID: callerID, ReceiverID: in.ReceiverID,
		Status: StatusCreated, CreatedAt: now, UpdatedAt: now,
	}
	return s.repo.Create(ctx, i, in.ClientRequestID)
}

// Start is PulseStart (spec §15): the sender began holding. Decides
// Live vs. Push once, from a real presence check against
// realtime-gateway (spec §4: never re-decided mid-interaction, no
// "best effort" blend) - fails closed to Push if presence itself can't
// be checked, since an honest "assume offline" is safer than a live
// attempt that silently goes nowhere. Live delivers a best-effort
// real-time push immediately; Push makes no delivery attempt yet at
// all - the single push notification goes out once, at Stop, carrying
// the real final duration (spec §4.2).
func (s *Service) Start(ctx context.Context, presence Presence, callerID, id string) (Interaction, error) {
	i, err := s.repo.Get(ctx, id)
	if err != nil {
		return Interaction{}, err
	}
	if i.SenderID != callerID {
		return Interaction{}, ErrForbidden
	}
	if i.Status != StatusCreated {
		return Interaction{}, ErrInvalidTransition
	}

	deliveryMode := DeliveryPush
	if online, err := presence.IsOnline(ctx, i.ReceiverID); err == nil && online {
		deliveryMode = DeliveryLive
	}

	now := time.Now().UTC()
	started, err := s.repo.Start(ctx, id, now, deliveryMode)
	if err != nil {
		return Interaction{}, err
	}
	if deliveryMode == DeliveryLive {
		s.publishPush(ctx, "pulse.started", started.ReceiverID, map[string]any{
			"interactionId": started.ID, "senderId": started.SenderID, "startedAt": now.Format(time.RFC3339Nano),
		})
	}
	return started, nil
}

// Stop is PulseStop (spec §15): duration is computed server-side from
// the row's own StartedAt, never a client-submitted value (spec §78).
// Live interactions get a second best-effort realtime push; Push
// interactions get their one real push notification here, now that the
// final duration is known.
func (s *Service) Stop(ctx context.Context, notifier Notifier, callerID, id string) (Interaction, error) {
	i, err := s.repo.Get(ctx, id)
	if err != nil {
		return Interaction{}, err
	}
	if i.SenderID != callerID {
		return Interaction{}, ErrForbidden
	}
	if i.Status != StatusStarted {
		return Interaction{}, ErrInvalidTransition
	}
	now := time.Now().UTC()
	completed, err := s.repo.Stop(ctx, id, now)
	if err != nil {
		return Interaction{}, err
	}
	durationMs := 0
	if completed.DurationMs != nil {
		durationMs = *completed.DurationMs
	}
	switch completed.DeliveryMode {
	case DeliveryLive:
		s.publishPush(ctx, "pulse.stopped", completed.ReceiverID, map[string]any{
			"interactionId": completed.ID, "endedAt": now.Format(time.RFC3339Nano),
		})
	case DeliveryPush:
		// Best-effort, same as live delivery - a failed push request
		// (e.g. the receiver has no registered device) never fails the
		// underlying Stop call; the durable interaction record already
		// captured what happened.
		_ = notifier.NotifyPulseReceived(ctx, completed.ReceiverID, durationMs)
	}
	s.trackCompleted(ctx, completed, durationMs)
	return completed, nil
}

func (s *Service) Get(ctx context.Context, callerID, id string) (Interaction, error) {
	i, err := s.repo.Get(ctx, id)
	if err != nil {
		return Interaction{}, err
	}
	if i.SenderID != callerID && i.ReceiverID != callerID {
		return Interaction{}, ErrForbidden
	}
	return i, nil
}

func (s *Service) ListMine(ctx context.Context, callerID string, limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListForUser(ctx, callerID, limit)
}

type pulsePushPayload struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

func (s *Service) publishPush(ctx context.Context, eventType, receiverID string, data map[string]any) {
	payload, err := json.Marshal(pulsePushPayload{Type: eventType, Data: data})
	if err != nil {
		return
	}
	// Best-effort: a failed publish (receiver offline, Redis hiccup)
	// never fails the underlying Start/Stop call - the durable record
	// is the source of truth, live delivery is a bonus this phase.
	_ = s.realtime.ToUser(ctx, receiverID, payload)
}

func (s *Service) trackCompleted(ctx context.Context, i Interaction, durationMs int) {
	_ = s.analytics.Track(ctx, "pulse_completed", i.SenderID, map[string]any{
		"interactionId": i.ID, "receiverId": i.ReceiverID, "durationMs": durationMs, "deliveryMode": string(i.DeliveryMode),
	})
}
