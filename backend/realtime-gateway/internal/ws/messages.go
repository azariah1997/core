package ws

import "encoding/json"

// clientMessage is what a connected client may send. Only one of
// Channel/TargetUserID is meaningful, depending on Type.
type clientMessage struct {
	Type         string          `json:"type"` // subscribe, unsubscribe, publish, direct
	Channel      string          `json:"channel,omitempty"`
	TargetUserID string          `json:"targetUserId,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
}

// serverMessage is what the server ever sends. Fields are populated per
// Type; the rest stay zero and are omitted from the JSON.
type serverMessage struct {
	Type         string          `json:"type"` // connected, subscribed, unsubscribed, message, direct, error
	ConnectionID string          `json:"connectionId,omitempty"`
	UserID       string          `json:"userId,omitempty"`
	DeviceID     string          `json:"deviceId,omitempty"`
	Channel      string          `json:"channel,omitempty"`
	FromUserID   string          `json:"fromUserId,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Message      string          `json:"message,omitempty"`
}
