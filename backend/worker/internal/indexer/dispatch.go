// Package indexer is the roadmap's own example made real: "user.updated
// -> search index worker." It polls outbox_events for the specific event
// types it knows how to turn into a searchidx.Document and applies them,
// leaving every other event's row untouched (published_at stays NULL) so
// a future Kafka relay - which doesn't exist yet, same gap noted since
// Phase 2 - can still find and process every event, not just the ones
// this indexer claims responsibility for.
//
// Indexed documents only ever contain what the source event's payload
// already carries (a handful of key fields, not full entity content) -
// worker is a separate Go module from core-api and can't import its
// internal domain packages, and re-deriving full records via a
// service-to-service HTTP call would need auth infrastructure this
// platform hasn't built yet. A deliberate, documented scope-down, not an
// oversight.
package indexer

import (
	"encoding/json"

	"github.com/example/core-platform/packages/go/platformkit/searchidx"
)

type action int

const (
	actionSkip action = iota
	actionIndex
	actionDelete
)

type dispatch struct {
	action  action
	doc     searchidx.Document
	delType string
	delID   string
}

// recognizedEventTypes is exactly what PollOnce filters its SELECT for.
var recognizedEventTypes = []string{
	"user.created", "user.updated", "user.deactivated", "user.deleted",
	"application.created", "application.updated",
	"message.sent",
}

func buildDispatch(eventType string, payload []byte) (dispatch, error) {
	switch eventType {
	case "user.created", "user.updated", "user.deactivated":
		var p struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Status      string `json:"status"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return dispatch{}, err
		}
		return dispatch{action: actionIndex, doc: searchidx.Document{
			Type: "user", ID: p.ID, Fields: map[string]any{"displayName": p.DisplayName, "status": p.Status},
		}}, nil

	case "user.deleted":
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return dispatch{}, err
		}
		return dispatch{action: actionDelete, delType: "user", delID: p.ID}, nil

	case "application.created", "application.updated":
		var p struct {
			ID     string `json:"id"`
			Slug   string `json:"slug"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return dispatch{}, err
		}
		return dispatch{action: actionIndex, doc: searchidx.Document{
			Type: "application", ID: p.ID, Fields: map[string]any{"slug": p.Slug, "name": p.Name, "status": p.Status},
		}}, nil

	case "message.sent":
		var p struct {
			ID             string `json:"id"`
			ConversationID string `json:"conversationId"`
			SenderID       string `json:"senderId"`
			Type           string `json:"type"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return dispatch{}, err
		}
		return dispatch{action: actionIndex, doc: searchidx.Document{
			Type: "message", ID: p.ID,
			Fields: map[string]any{"conversationId": p.ConversationID, "senderId": p.SenderID, "type": p.Type},
		}}, nil

	default:
		return dispatch{action: actionSkip}, nil
	}
}
