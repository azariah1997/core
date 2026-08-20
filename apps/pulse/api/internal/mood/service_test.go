package mood_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/mood"
	"github.com/example/core-platform/apps/pulse/api/internal/mood/memory"
)

type fakeConnections struct {
	conns []mood.ConnectionRef
	err   error
}

func (f fakeConnections) ListMine(ctx context.Context, callerID string) ([]mood.ConnectionRef, error) {
	return f.conns, f.err
}

type fakeBond struct {
	partnerID string
	hasBond   bool
	err       error
}

func (f fakeBond) MyActiveBond(ctx context.Context, callerID string) (mood.BondRef, error) {
	if f.err != nil {
		return mood.BondRef{}, f.err
	}
	if !f.hasBond {
		return mood.BondRef{}, mood.ErrNoBond
	}
	return mood.BondRef{UserAID: callerID, UserBID: f.partnerID}, nil
}

type fakeCircles struct {
	members map[string][]string
	err     error
}

func (f fakeCircles) ListMembers(ctx context.Context, callerID, circleID string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.members[circleID], nil
}

type fakeAnalytics struct {
	mu     sync.Mutex
	tracks []string
}

func (f *fakeAnalytics) Track(ctx context.Context, eventName, userID string, properties map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tracks = append(f.tracks, eventName+":"+userID)
	return nil
}

func newService(bond mood.Bond) *mood.Service {
	svc, _ := newServiceWithAnalytics(bond)
	return svc
}

func newServiceWithAnalytics(bond mood.Bond) (*mood.Service, *fakeAnalytics) {
	analytics := &fakeAnalytics{}
	return mood.NewService(memory.New(), bond, analytics), analytics
}

func TestSetRejectsAnInvalidEmoji(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: "🚀", Audience: mood.AudiencePrivate})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unrecognized emoji, got %v", err)
	}
}

func TestSetRejectsAnInvalidAudience(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: "everyone"})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for an unrecognized audience, got %v", err)
	}
}

func TestSetRequiresCustomUserIDsForCustomAudience(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudienceCustomUsers})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError when custom_users has no customUserIds, got %v", err)
	}
}

func TestSetRejectsCustomUserIDsOnANonCustomAudience(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate, CustomUserIDs: []string{"user-2"}})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for customUserIds on a non-custom audience, got %v", err)
	}
}

func TestSetPartnerOnlyResolvesToTheRealActiveBondPartner(t *testing.T) {
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"})
	m, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiHeart, Audience: mood.AudiencePartnerOnly})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("partner-1", time.Now().UTC()) {
		t.Fatalf("expected the real bond partner to be able to view a partner_only Mood")
	}
	if m.CanView("stranger", time.Now().UTC()) {
		t.Fatalf("expected a non-partner to be denied a partner_only Mood")
	}
}

func TestSetPartnerOnlyWithNoActiveBondIsVisibleToNoOneButTheOwner(t *testing.T) {
	svc := newService(fakeBond{hasBond: false})
	m, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiHeart, Audience: mood.AudiencePartnerOnly})
	if err != nil {
		t.Fatalf("expected Set to succeed even with no active bond, got %v", err)
	}
	if !m.CanView("caller-1", time.Now().UTC()) {
		t.Fatalf("expected the owner to always see their own Mood")
	}
	if m.CanView("anyone-else", time.Now().UTC()) {
		t.Fatalf("expected an unpartnered partner_only Mood to be visible to no one else")
	}
}

func TestSetCloseFriendsFiltersToActiveCloseFriendClassification(t *testing.T) {
	conns := fakeConnections{conns: []mood.ConnectionRef{
		{OtherUserID: "close-1", Status: "active", Classification: "close_friend"},
		{OtherUserID: "plain-friend", Status: "active", Classification: "friend"},
		{OtherUserID: "inactive-close", Status: "pending", Classification: "close_friend"},
	}}
	svc := newService(fakeBond{})
	m, err := svc.Set(context.Background(), conns, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiFire, Audience: mood.AudienceCloseFriends})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("close-1", time.Now().UTC()) {
		t.Fatalf("expected an active close friend to see a close_friends Mood")
	}
	if m.CanView("plain-friend", time.Now().UTC()) {
		t.Fatalf("expected a plain (non-close) friend to be excluded from a close_friends Mood")
	}
	if m.CanView("inactive-close", time.Now().UTC()) {
		t.Fatalf("expected a non-active close friend connection to be excluded")
	}
}

func TestSetAllConnectionsIncludesEveryActiveConnectionRegardlessOfClassification(t *testing.T) {
	conns := fakeConnections{conns: []mood.ConnectionRef{
		{OtherUserID: "close-1", Status: "active", Classification: "close_friend"},
		{OtherUserID: "plain-friend", Status: "active", Classification: "friend"},
		{OtherUserID: "removed", Status: "ended", Classification: "friend"},
	}}
	svc := newService(fakeBond{})
	m, err := svc.Set(context.Background(), conns, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiWave, Audience: mood.AudienceAllConnections})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("close-1", time.Now().UTC()) || !m.CanView("plain-friend", time.Now().UTC()) {
		t.Fatalf("expected every active connection regardless of classification to see an all_connections Mood")
	}
	if m.CanView("removed", time.Now().UTC()) {
		t.Fatalf("expected an ended connection to be excluded")
	}
}

func TestSetCustomUsersOnlyIncludesRealActiveConnections(t *testing.T) {
	conns := fakeConnections{conns: []mood.ConnectionRef{
		{OtherUserID: "friend-1", Status: "active", Classification: "friend"},
	}}
	svc := newService(fakeBond{})
	m, err := svc.Set(context.Background(), conns, fakeCircles{}, "caller-1", "UTC", mood.SetInput{
		Emoji: mood.EmojiMoon, Audience: mood.AudienceCustomUsers, CustomUserIDs: []string{"friend-1", "not-actually-connected"},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("friend-1", time.Now().UTC()) {
		t.Fatalf("expected a real active connection supplied in customUserIds to see the Mood")
	}
	if m.CanView("not-actually-connected", time.Now().UTC()) {
		t.Fatalf("expected an ID that isn't a real active connection to never be granted visibility, regardless of what the client requested")
	}
}

func TestSetPrivateGrantsOnlyTheOwner(t *testing.T) {
	conns := fakeConnections{conns: []mood.ConnectionRef{{OtherUserID: "friend-1", Status: "active", Classification: "close_friend"}}}
	svc := newService(fakeBond{hasBond: true, partnerID: "partner-1"})
	m, err := svc.Set(context.Background(), conns, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSleep, Audience: mood.AudiencePrivate})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("caller-1", time.Now().UTC()) {
		t.Fatalf("expected the owner to always see their own private Mood")
	}
	if m.CanView("friend-1", time.Now().UTC()) || m.CanView("partner-1", time.Now().UTC()) {
		t.Fatalf("expected a private Mood to be visible to no one but the owner")
	}
}

func TestGetHidesAMoodFromSomeoneOutsideItsAudience(t *testing.T) {
	svc := newService(fakeBond{})
	if _, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiRain, Audience: mood.AudiencePrivate}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.Get(context.Background(), "stranger", "caller-1"); !errors.Is(err, mood.ErrNotFound) {
		t.Fatalf("expected ErrNotFound (not a distinct forbidden error) for a viewer outside the audience, got %v", err)
	}
}

func TestGetAllowsAnAudienceMemberToView(t *testing.T) {
	conns := fakeConnections{conns: []mood.ConnectionRef{{OtherUserID: "friend-1", Status: "active", Classification: "friend"}}}
	svc := newService(fakeBond{})
	if _, err := svc.Set(context.Background(), conns, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiHug, Audience: mood.AudienceAllConnections}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := svc.Get(context.Background(), "friend-1", "caller-1")
	if err != nil {
		t.Fatalf("expected an audience member to view the Mood, got %v", err)
	}
	if got.Emoji != mood.EmojiHug {
		t.Fatalf("expected the real emoji, got %v", got.Emoji)
	}
}

func TestMyMoodReturnsNotFoundWhenNoneIsSet(t *testing.T) {
	svc := newService(fakeBond{})
	if _, err := svc.MyMood(context.Background(), "caller-1"); !errors.Is(err, mood.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestClearRemovesTheMood(t *testing.T) {
	svc := newService(fakeBond{})
	if _, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.Clear(context.Background(), "caller-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := svc.MyMood(context.Background(), "caller-1"); !errors.Is(err, mood.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Clear, got %v", err)
	}
}

func TestSetPreservesCreatedAtWhenUpdatingAnAlreadyActiveMood(t *testing.T) {
	svc := newService(fakeBond{})
	first, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate})
	if err != nil {
		t.Fatalf("first set: %v", err)
	}
	second, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiFire, Audience: mood.AudiencePrivate})
	if err != nil {
		t.Fatalf("second set: %v", err)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("expected CreatedAt to be preserved across an update to the same day's Mood, got first=%v second=%v", first.CreatedAt, second.CreatedAt)
	}
	if second.Emoji != mood.EmojiFire {
		t.Fatalf("expected the emoji to actually update, got %v", second.Emoji)
	}
}

func TestSetRecordsADurableMoodSetAnalyticsEvent(t *testing.T) {
	svc, analytics := newServiceWithAnalytics(fakeBond{})
	if _, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate}); err != nil {
		t.Fatalf("set: %v", err)
	}
	analytics.mu.Lock()
	defer analytics.mu.Unlock()
	if len(analytics.tracks) != 1 || analytics.tracks[0] != "mood_set:caller-1" {
		t.Fatalf("expected a mood_set analytics event attributed to caller-1, got %v", analytics.tracks)
	}
}

func TestClearRecordsADurableMoodClearedAnalyticsEvent(t *testing.T) {
	svc, analytics := newServiceWithAnalytics(fakeBond{})
	if _, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.Clear(context.Background(), "caller-1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	analytics.mu.Lock()
	defer analytics.mu.Unlock()
	found := false
	for _, tr := range analytics.tracks {
		if tr == "mood_cleared:caller-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a mood_cleared analytics event, got %v", analytics.tracks)
	}
}

func TestSetRequiresCircleIDForSelectedCirclesAudience(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudienceSelectedCircles})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError when selected_circles has no circleId, got %v", err)
	}
}

func TestSetRejectsCircleIDOnANonCircleAudience(t *testing.T) {
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate, CircleID: "circle-1"})
	var validationErr *mood.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected a ValidationError for circleId on a non-circle audience, got %v", err)
	}
}

func TestSetSelectedCirclesResolvesToTheRealCircleMembership(t *testing.T) {
	circles := fakeCircles{members: map[string][]string{"circle-1": {"caller-1", "member-1", "member-2"}}}
	svc := newService(fakeBond{})
	m, err := svc.Set(context.Background(), fakeConnections{}, circles, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiHug, Audience: mood.AudienceSelectedCircles, CircleID: "circle-1"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !m.CanView("member-1", time.Now().UTC()) || !m.CanView("member-2", time.Now().UTC()) {
		t.Fatalf("expected every real Circle member to see a selected_circles Mood")
	}
	if m.CanView("not-a-member", time.Now().UTC()) {
		t.Fatalf("expected a non-member to be denied")
	}
	if m.CircleID == nil || *m.CircleID != "circle-1" {
		t.Fatalf("expected CircleID to be recorded for the owner's own management view, got %v", m.CircleID)
	}
}

func TestSetSelectedCirclesFailsWhenTheCallerIsNotAMemberOfTheReferencedCircle(t *testing.T) {
	circles := fakeCircles{err: errors.New("forbidden: not a member of this group")}
	svc := newService(fakeBond{})
	_, err := svc.Set(context.Background(), fakeConnections{}, circles, "caller-1", "UTC", mood.SetInput{Emoji: mood.EmojiHug, Audience: mood.AudienceSelectedCircles, CircleID: "someone-elses-circle"})
	if err == nil {
		t.Fatalf("expected Set to fail honestly when the caller can't actually see the referenced Circle's membership")
	}
}

func TestExpiryIsTheNextMidnightInTheCallersOwnTimezone(t *testing.T) {
	svc := newService(fakeBond{})
	loc, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Skipf("tzdata unavailable in this environment: %v", err)
	}
	m, err := svc.Set(context.Background(), fakeConnections{}, fakeCircles{}, "caller-1", "Pacific/Auckland", mood.SetInput{Emoji: mood.EmojiSun, Audience: mood.AudiencePrivate})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	localExpiry := m.ExpiresAt.In(loc)
	if h, mi, s := localExpiry.Clock(); h != 0 || mi != 0 || s != 0 {
		t.Fatalf("expected ExpiresAt to land exactly on local midnight in Pacific/Auckland, got %02d:%02d:%02d", h, mi, s)
	}
}
