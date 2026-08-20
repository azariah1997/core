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
	return s.createInteraction(ctx, core, TypePulse, callerID, in)
}

// createInteraction is Create's actual logic, parametrized by Type so
// Knock (and Phase 11's Custom Signals) share the exact same
// authorization/rate-limit/idempotency path rather than duplicating it -
// every Type currently shares one rate-limit budget, since they're all
// the same "sender reaches out" action from the receiver's perspective.
func (s *Service) createInteraction(ctx context.Context, core CoreRelationships, typ Type, callerID string, in CreateInput) (Interaction, error) {
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
		ID: uuid.NewString(), Type: typ, SenderID: callerID, ReceiverID: in.ReceiverID,
		Status: StatusCreated, CreatedAt: now, UpdatedAt: now,
	}
	if in.InResponseToID != "" {
		i.InResponseToID = &in.InResponseToID
	}
	if in.Pattern != "" {
		i.Pattern = &in.Pattern
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
		s.publishPush(ctx, string(started.Type)+".started", started.ReceiverID, map[string]any{
			"interactionId": started.ID, "senderId": started.SenderID, "startedAt": now.Format(time.RFC3339Nano),
		})
	}
	_ = s.analytics.Track(ctx, string(started.Type)+"_started", started.SenderID, map[string]any{
		"interactionId": started.ID, "receiverId": started.ReceiverID, "deliveryMode": string(deliveryMode),
	})
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
		s.publishPush(ctx, string(completed.Type)+".stopped", completed.ReceiverID, map[string]any{
			"interactionId": completed.ID, "endedAt": now.Format(time.RFC3339Nano),
		})
	case DeliveryPush:
		// Best-effort, same as live delivery - a failed push request
		// (e.g. the receiver has no registered device) never fails the
		// underlying Stop call; the durable interaction record already
		// captured what happened.
		category, title, body, data := notificationContent(completed, durationMs)
		_ = notifier.Notify(ctx, completed.ReceiverID, category, title, body, data)
	}
	s.trackCompleted(ctx, completed, durationMs)
	return completed, nil
}

// notificationContent picks the push category/title/body/data per Type -
// the one place Pulse and Knock's Stop-time behavior actually differs.
// Content is deliberately generic (spec §72's Private-tier default),
// never naming the sender.
func notificationContent(i Interaction, durationMs int) (category, title, body string, data map[string]any) {
	switch i.Type {
	case TypeKnock:
		pattern := string(KnockPatternDoubleTap)
		if i.Pattern != nil {
			pattern = *i.Pattern
		}
		return "knock_received", "Knock", "You received a Knock", map[string]any{"pattern": pattern}
	default:
		return "pulse_received", "Pulse", "You received a Pulse", map[string]any{"durationMs": durationMs}
	}
}

// PulseBack is the fundamental social loop (spec §17): "A → Pulse, B →
// feels it, B → Pulse Back, A → feels response" - faster than opening a
// messaging interface, so this is one API call, not three. Internally
// it's exactly a normal Create+Start+Stop targeting the original
// sender, linked via InResponseToID - reusing those three methods
// directly (rather than duplicating their authorization, rate-limiting,
// and presence-check logic) means a Pulse Back gets every real
// guarantee an ordinary Pulse does for free: connection/block checks,
// the same rate limit, a fresh presence check for whether *this*
// direction is live or push right now (independent of what the
// original Pulse's delivery mode was).
func (s *Service) PulseBack(ctx context.Context, core CoreRelationships, presence Presence, notifier Notifier, callerID, originalID string) (Interaction, error) {
	original, err := s.repo.Get(ctx, originalID)
	if err != nil {
		return Interaction{}, err
	}
	if original.ReceiverID != callerID {
		return Interaction{}, ErrForbidden
	}
	if original.Status != StatusCompleted {
		return Interaction{}, ErrInvalidTransition
	}

	created, err := s.Create(ctx, core, callerID, CreateInput{ReceiverID: original.SenderID, InResponseToID: original.ID})
	if err != nil {
		return Interaction{}, err
	}
	if _, err := s.Start(ctx, presence, callerID, created.ID); err != nil {
		return Interaction{}, err
	}
	completed, err := s.Stop(ctx, notifier, callerID, created.ID)
	if err != nil {
		return Interaction{}, err
	}

	// Measure "Pulse received -> Pulse Back" (spec §6's own named
	// metric): the gap between when the original Pulse actually reached
	// this responder (its EndedAt - the moment the felt gesture
	// finished) and the moment they finished responding.
	if original.EndedAt != nil {
		responseLatencyMs := int(completed.CreatedAt.Sub(*original.EndedAt).Milliseconds())
		_ = s.analytics.Track(ctx, "pulse_back", callerID, map[string]any{
			"interactionId": completed.ID, "inResponseToId": original.ID, "responseLatencyMs": responseLatencyMs,
		})
	}
	return completed, nil
}

// Knock is spec §18: a short predefined haptic pattern, distinct from a
// held Pulse - "quicker, lighter... a nudge, not a hold." Internally it's
// Create+Start+Stop back-to-back against the shared interaction state
// machine (the same reuse PulseBack established), typed TypeKnock with a
// Pattern carried through from creation - the caller feels a fixed
// pattern, never a variable duration the way a Pulse does, so Stop
// follows Start immediately rather than waiting on any client signal.
func (s *Service) Knock(ctx context.Context, core CoreRelationships, presence Presence, notifier Notifier, callerID string, in KnockInput) (Interaction, error) {
	pattern := in.Pattern
	if pattern == "" {
		pattern = KnockPatternDoubleTap
	}
	if !pattern.valid() {
		return Interaction{}, &ValidationError{Message: "invalid knock pattern"}
	}

	created, err := s.createInteraction(ctx, core, TypeKnock, callerID, CreateInput{
		ReceiverID: in.ReceiverID,
		Pattern:    string(pattern),
	})
	if err != nil {
		return Interaction{}, err
	}
	if _, err := s.Start(ctx, presence, callerID, created.ID); err != nil {
		return Interaction{}, err
	}
	return s.Stop(ctx, notifier, callerID, created.ID)
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
	_ = s.analytics.Track(ctx, string(i.Type)+"_completed", i.SenderID, map[string]any{
		"interactionId": i.ID, "receiverId": i.ReceiverID, "durationMs": durationMs, "deliveryMode": string(i.DeliveryMode),
	})
}
