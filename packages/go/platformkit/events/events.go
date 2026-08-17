package events

import "time"

type Envelope struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Version       int            `json:"version"`
	Source        string         `json:"source"`
	CorrelationID string         `json:"correlationId"`
	OccurredAt    time.Time      `json:"occurredAt"`
	Data          map[string]any `json:"data"`
}
