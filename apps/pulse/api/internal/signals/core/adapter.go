// Package core adapts a per-caller *coresdk.Client onto
// signals.CoreRelationships - duplicated from pulse-interactions' own
// RelationshipsAdapter of the same shape rather than imported (this
// codebase's consumer-defined-interface convention).
package core

import (
	"context"
	"net/url"

	"github.com/example/core-platform/apps/pulse/api/internal/signals"
	"github.com/example/core-platform/packages/go/coresdk"
)

type RelationshipsAdapter struct {
	client *coresdk.Client
	appID  string
}

func NewRelationshipsAdapter(client *coresdk.Client, appID string) RelationshipsAdapter {
	return RelationshipsAdapter{client: client, appID: appID}
}

func (a RelationshipsAdapter) ListMine(ctx context.Context, relType string) ([]signals.RelationshipRef, error) {
	var out struct {
		Items []struct {
			RequesterID string `json:"requesterUserId"`
			TargetID    string `json:"targetUserId"`
			Status      string `json:"status"`
		} `json:"items"`
	}
	path := "/v1/relationships?appId=" + url.QueryEscape(a.appID) + "&type=" + url.QueryEscape(relType)
	if err := a.client.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	refs := make([]signals.RelationshipRef, 0, len(out.Items))
	for _, item := range out.Items {
		refs = append(refs, signals.RelationshipRef{RequesterID: item.RequesterID, TargetID: item.TargetID, Status: item.Status})
	}
	return refs, nil
}
