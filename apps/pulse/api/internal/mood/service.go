package mood

import (
	"context"
	"errors"
	"time"
)

type Service struct {
	repo      Repository
	bond      Bond
	analytics Analytics
}

// bond and analytics are both fixed, service-level dependencies (unlike
// Connections below) - MyActiveBond reads only Pulse's own Postgres,
// and analytics ingest needs no caller token either, so neither needs a
// per-request factory.
func NewService(repo Repository, bond Bond, analytics Analytics) *Service {
	return &Service{repo: repo, bond: bond, analytics: analytics}
}

// Set is Today's Mood's one mutation - creates or replaces the caller's
// current Mood, resolving Audience into a concrete AllowedViewerIDs
// snapshot right now, under the caller's own real connections/bond/
// Circle state (see Mood's own doc comment for why this can't happen
// later, per-viewer). connections and circles are both per-request
// (built from the caller's own authenticated client, like every other
// module's CoreRelationships), unlike bond above.
func (s *Service) Set(ctx context.Context, connections Connections, circles Circles, callerID, timezone string, in SetInput) (Mood, error) {
	if err := in.Validate(); err != nil {
		return Mood{}, err
	}

	now := time.Now().UTC()
	createdAt := now
	if existing, err := s.repo.Get(ctx, callerID); err == nil {
		// Updating an already-active Mood (spec §103's "Mood can be
		// updated") preserves when it was first set today; an expired
		// or never-set Mood starts fresh - repo.Get treats "expired" and
		// "never set" identically (see Repository's own doc comment).
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, ErrNotFound) {
		return Mood{}, err
	}

	allowed, err := s.resolveAudience(ctx, connections, circles, callerID, in)
	if err != nil {
		return Mood{}, err
	}

	m := Mood{
		UserID: callerID, Emoji: in.Emoji, Audience: in.Audience, AllowedViewerIDs: allowed,
		CreatedAt: createdAt, ExpiresAt: nextLocalMidnight(now, timezone), UpdatedAt: now,
	}
	if in.Audience == AudienceSelectedCircles {
		circleID := in.CircleID
		m.CircleID = &circleID
	}
	saved, err := s.repo.Set(ctx, m)
	if err != nil {
		return Mood{}, err
	}
	_ = s.analytics.Track(ctx, "mood_set", callerID, map[string]any{
		"emoji": string(saved.Emoji), "audience": string(saved.Audience), "audienceSize": len(saved.AllowedViewerIDs),
	})
	return saved, nil
}

func (s *Service) resolveAudience(ctx context.Context, connections Connections, circles Circles, callerID string, in SetInput) ([]string, error) {
	switch in.Audience {
	case AudiencePartnerOnly:
		b, err := s.bond.MyActiveBond(ctx, callerID)
		if err != nil {
			if errors.Is(err, ErrNoBond) {
				// No active bond right now - the Mood is still created,
				// just visible to no one until the caller bonds with
				// someone (Set never fails based on current audience
				// membership).
				return nil, nil
			}
			return nil, err
		}
		return []string{b.otherUser(callerID)}, nil

	case AudienceCloseFriends, AudienceAllConnections:
		conns, err := connections.ListMine(ctx, callerID)
		if err != nil {
			return nil, err
		}
		var allowed []string
		for _, c := range conns {
			if c.Status != "active" {
				continue
			}
			if in.Audience == AudienceCloseFriends && c.Classification != "close_friend" {
				continue
			}
			allowed = append(allowed, c.OtherUserID)
		}
		return allowed, nil

	case AudienceCustomUsers:
		conns, err := connections.ListMine(ctx, callerID)
		if err != nil {
			return nil, err
		}
		active := make(map[string]bool, len(conns))
		for _, c := range conns {
			if c.Status == "active" {
				active[c.OtherUserID] = true
			}
		}
		var allowed []string
		for _, id := range in.CustomUserIDs {
			// Only ever grants visibility to a real, currently-active
			// connection - never an arbitrary stranger ID a client
			// might supply (the same authorization discipline
			// pulse-interactions' own connection check enforces).
			if active[id] {
				allowed = append(allowed, id)
			}
		}
		return allowed, nil

	case AudienceSelectedCircles:
		members, err := circles.ListMembers(ctx, callerID, in.CircleID)
		if err != nil {
			return nil, err
		}
		var allowed []string
		for _, id := range members {
			// The circle membership itself already includes the caller
			// (Core's real groups.Create seeds the creator as a manager
			// member) - exclude them from AllowedViewerIDs since
			// CanView already grants the owner unconditionally, and
			// nothing else should ever compare a userID to itself here.
			if id != callerID {
				allowed = append(allowed, id)
			}
		}
		return allowed, nil

	default: // AudiencePrivate
		return nil, nil
	}
}

// MyMood returns the caller's own current Mood.
func (s *Service) MyMood(ctx context.Context, callerID string) (Mood, error) {
	return s.repo.Get(ctx, callerID)
}

// Get returns ownerID's Mood as seen by viewerID - ErrNotFound both when
// no Mood exists and when one exists but viewerID isn't in its audience,
// deliberately indistinguishable (see Mood.CanView's own doc comment:
// never leak whether a Mood exists to someone not allowed to see it).
func (s *Service) Get(ctx context.Context, viewerID, ownerID string) (Mood, error) {
	m, err := s.repo.Get(ctx, ownerID)
	if err != nil {
		return Mood{}, err
	}
	if !m.CanView(viewerID, time.Now().UTC()) {
		return Mood{}, ErrNotFound
	}
	return m, nil
}

func (s *Service) Clear(ctx context.Context, callerID string) error {
	if err := s.repo.Clear(ctx, callerID); err != nil {
		return err
	}
	_ = s.analytics.Track(ctx, "mood_cleared", callerID, nil)
	return nil
}

// nextLocalMidnight is Mood's expiry (spec §26: "expires at user-local
// day boundary ... store timezone correctly"). Falls back to UTC for an
// empty or unrecognized timezone - Core's own users.timezone column
// defaults to 'UTC', so this should be rare, but a Mood should never
// fail to be set over a bad timezone string.
func nextLocalMidnight(now time.Time, timezone string) time.Time {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	y, mo, d := local.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, loc).AddDate(0, 0, 1)
}
