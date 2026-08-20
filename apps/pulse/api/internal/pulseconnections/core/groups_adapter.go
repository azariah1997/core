package core

import (
	"context"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	"github.com/example/core-platform/packages/go/coresdk"
)

// GroupsAdapter adapts a per-caller *coresdk.Client onto
// pulseconnections.CoreGroups - Core's real /v1/groups routes
// (backend/core-api/internal/groups), the platform's generic grouping
// primitive Circles are built directly on top of, never reimplemented.
type GroupsAdapter struct {
	client *coresdk.Client
	appID  string
}

// NewGroupsAdapter wraps a *coresdk.Client already authenticated as the
// current caller - same per-caller discipline as Adapter above, since
// every groups call must be made as the real caller for Core's own
// manager-based authorization to apply. appID is Pulse's own real Core
// Application ID - Core's groups aren't otherwise scoped per
// application on read (see ListMine's own doc comment), so this
// adapter both sends it on Create and filters by it on ListMine.
func NewGroupsAdapter(client *coresdk.Client, appID string) GroupsAdapter {
	return GroupsAdapter{client: client, appID: appID}
}

type groupResponse struct {
	ID        string `json:"id"`
	AppID     string `json:"appId"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (g groupResponse) toCircle() pulseconnections.Circle {
	createdAt, _ := time.Parse(timeFormat, g.CreatedAt)
	updatedAt, _ := time.Parse(timeFormat, g.UpdatedAt)
	return pulseconnections.Circle{ID: g.ID, Name: g.Name, Status: g.Status, CreatedAt: createdAt, UpdatedAt: updatedAt}
}

type memberResponse struct {
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	IsManager bool   `json:"isManager"`
	CreatedAt string `json:"createdAt"`
}

func (m memberResponse) toMember() pulseconnections.CircleMember {
	createdAt, _ := time.Parse(timeFormat, m.CreatedAt)
	return pulseconnections.CircleMember{UserID: m.UserID, Role: m.Role, IsManager: m.IsManager, CreatedAt: createdAt}
}

func (a GroupsAdapter) Create(ctx context.Context, name string) (pulseconnections.Circle, error) {
	var out groupResponse
	body := map[string]any{"appId": a.appID, "name": name}
	if err := a.client.Do(ctx, "POST", "/v1/groups", body, &out); err != nil {
		return pulseconnections.Circle{}, err
	}
	return out.toCircle(), nil
}

// ListMine calls Core's real GET /v1/groups, which returns every group
// the caller belongs to across every application (no appId parameter
// exists on that route) - filtered here to Pulse's own AppID so a
// Circle from an unrelated Core application never leaks into Pulse's
// own Circle list.
func (a GroupsAdapter) ListMine(ctx context.Context) ([]pulseconnections.Circle, error) {
	var out struct {
		Items []groupResponse `json:"items"`
	}
	if err := a.client.Do(ctx, "GET", "/v1/groups", nil, &out); err != nil {
		return nil, err
	}
	circles := make([]pulseconnections.Circle, 0, len(out.Items))
	for _, g := range out.Items {
		if g.AppID != a.appID {
			continue
		}
		circles = append(circles, g.toCircle())
	}
	return circles, nil
}

func (a GroupsAdapter) ListMembers(ctx context.Context, circleID string) ([]pulseconnections.CircleMember, error) {
	var out struct {
		Items []memberResponse `json:"items"`
	}
	if err := a.client.Do(ctx, "GET", "/v1/groups/"+circleID+"/members", nil, &out); err != nil {
		return nil, err
	}
	members := make([]pulseconnections.CircleMember, 0, len(out.Items))
	for _, m := range out.Items {
		members = append(members, m.toMember())
	}
	return members, nil
}

func (a GroupsAdapter) AddMember(ctx context.Context, circleID, userID string) (pulseconnections.CircleMember, error) {
	var out memberResponse
	body := map[string]any{"userId": userID}
	if err := a.client.Do(ctx, "POST", "/v1/groups/"+circleID+"/members", body, &out); err != nil {
		return pulseconnections.CircleMember{}, err
	}
	return out.toMember(), nil
}

func (a GroupsAdapter) RemoveMember(ctx context.Context, circleID, userID string) error {
	return a.client.Do(ctx, "DELETE", "/v1/groups/"+circleID+"/members/"+userID, nil, nil)
}
