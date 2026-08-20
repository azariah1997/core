// Package core adapts a per-caller *coresdk.Client onto
// pulseconnections.CoreRelationships - the real implementation used in
// production, calling Core's actual /v1/relationships routes
// (coresdk has no typed relationship methods yet, so this uses its
// Do escape hatch directly - see coresdk/operations.go's own header
// comment for which routes got typed wrappers and why relationships
// isn't one of them yet).
package core

import (
	"context"
	"net/url"
	"time"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseconnections"
	"github.com/example/core-platform/packages/go/coresdk"
)

type Adapter struct {
	client *coresdk.Client
	appID  string
}

// New wraps a *coresdk.Client already authenticated as the current
// caller (see pulseauth.ClientFromContext) - never a fixed
// service-level credential, since every relationships call must be
// made as the real caller for Core's own authorization to apply.
// appID is Pulse's own real Core Application ID (registered once via
// POST /v1/apps during Phase 1) - required on every relationships
// call, since Core scopes the relationship graph per application.
func New(client *coresdk.Client, appID string) *Adapter {
	return &Adapter{client: client, appID: appID}
}

type relationshipResponse struct {
	ID          string `json:"id"`
	RequesterID string `json:"requesterUserId"`
	TargetID    string `json:"targetUserId"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func (r relationshipResponse) toRef() pulseconnections.RelationshipRef {
	createdAt, _ := time.Parse(timeFormat, r.CreatedAt)
	updatedAt, _ := time.Parse(timeFormat, r.UpdatedAt)
	return pulseconnections.RelationshipRef{
		ID: r.ID, RequesterID: r.RequesterID, TargetID: r.TargetID, Status: r.Status,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func (a *Adapter) Request(ctx context.Context, targetUserID, relType string) (pulseconnections.RelationshipRef, error) {
	var out relationshipResponse
	body := map[string]any{"appId": a.appID, "targetUserId": targetUserID, "type": relType}
	if err := a.client.Do(ctx, "POST", "/v1/relationships", body, &out); err != nil {
		return pulseconnections.RelationshipRef{}, err
	}
	return out.toRef(), nil
}

func (a *Adapter) Accept(ctx context.Context, relationshipID string) (pulseconnections.RelationshipRef, error) {
	var out relationshipResponse
	if err := a.client.Do(ctx, "POST", "/v1/relationships/"+relationshipID+"/accept", nil, &out); err != nil {
		return pulseconnections.RelationshipRef{}, err
	}
	return out.toRef(), nil
}

func (a *Adapter) Decline(ctx context.Context, relationshipID string) (pulseconnections.RelationshipRef, error) {
	var out relationshipResponse
	if err := a.client.Do(ctx, "POST", "/v1/relationships/"+relationshipID+"/decline", nil, &out); err != nil {
		return pulseconnections.RelationshipRef{}, err
	}
	return out.toRef(), nil
}

func (a *Adapter) Remove(ctx context.Context, relationshipID string) error {
	return a.client.Do(ctx, "DELETE", "/v1/relationships/"+relationshipID, nil, nil)
}

func (a *Adapter) ListMine(ctx context.Context, relType string) ([]pulseconnections.RelationshipRef, error) {
	var out struct {
		Items []relationshipResponse `json:"items"`
	}
	path := "/v1/relationships?appId=" + url.QueryEscape(a.appID) + "&type=" + url.QueryEscape(relType)
	if err := a.client.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	refs := make([]pulseconnections.RelationshipRef, 0, len(out.Items))
	for _, item := range out.Items {
		refs = append(refs, item.toRef())
	}
	return refs, nil
}
