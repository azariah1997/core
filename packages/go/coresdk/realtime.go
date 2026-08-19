package coresdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// RealtimeMessage mirrors realtime-gateway's real wire format exactly
// (backend/realtime-gateway/internal/ws/messages.go's serverMessage) -
// Type is one of "connected", "subscribed", "unsubscribed", "message",
// "direct", "error"; the rest are populated per Type.
type RealtimeMessage struct {
	Type         string          `json:"type"`
	ConnectionID string          `json:"connectionId,omitempty"`
	UserID       string          `json:"userId,omitempty"`
	DeviceID     string          `json:"deviceId,omitempty"`
	Channel      string          `json:"channel,omitempty"`
	FromUserID   string          `json:"fromUserId,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Message      string          `json:"message,omitempty"`
}

// RealtimeConn is a live connection to realtime-gateway. Reconnection
// is deliberately the caller's responsibility (via RealtimeClient.Dial
// again) rather than automatic inside RealtimeConn - transparent
// auto-reconnect would silently replay Subscribe calls the caller made
// against the old connection, which needs application-level judgment
// about what "recovered" even means for a given channel.
type RealtimeConn struct {
	ws *websocket.Conn
}

// RealtimeClient dials realtime-gateway. Kept separate from Client
// (core-api's HTTP client) since it's a genuinely different service
// with its own base URL - the same split the platform itself has
// between core-api and realtime-gateway.
type RealtimeClient struct {
	baseURL string
	tokens  TokenSource
}

// NewRealtimeClient builds a client for the realtime-gateway instance
// at baseURL (e.g. "http://localhost:8090" - converted to ws:// or
// wss:// automatically).
func NewRealtimeClient(baseURL string, tokens TokenSource) *RealtimeClient {
	return &RealtimeClient{baseURL: baseURL, tokens: tokens}
}

// Dial opens a real WebSocket connection, exactly reproducing
// realtime-gateway's actual auth contract: access_token and deviceId as
// query parameters (see cmd/server/auth.go's wsAuthMiddleware) - both
// required, not just the token.
func (rc *RealtimeClient) Dial(ctx context.Context, deviceID string) (*RealtimeConn, error) {
	if deviceID == "" {
		return nil, fmt.Errorf("coresdk: deviceId is required to dial realtime-gateway")
	}
	token, err := rc.tokens.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("coresdk: obtain token for realtime dial: %w", err)
	}

	wsURL := strings.Replace(strings.Replace(rc.baseURL, "https://", "wss://", 1), "http://", "ws://", 1)
	u, err := url.Parse(strings.TrimRight(wsURL, "/") + "/ws")
	if err != nil {
		return nil, fmt.Errorf("coresdk: parse realtime URL: %w", err)
	}
	q := u.Query()
	q.Set("access_token", token)
	q.Set("deviceId", deviceID)
	u.RawQuery = q.Encode()

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("coresdk: realtime dial failed (status %d): %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("coresdk: realtime dial failed: %w", err)
	}
	return &RealtimeConn{ws: conn}, nil
}

// Subscribe joins channel - the client-side counterpart of clientMessage{Type: "subscribe"}.
func (c *RealtimeConn) Subscribe(channel string) error {
	return c.send(map[string]any{"type": "subscribe", "channel": channel})
}

// Unsubscribe leaves channel.
func (c *RealtimeConn) Unsubscribe(channel string) error {
	return c.send(map[string]any{"type": "unsubscribe", "channel": channel})
}

// Publish sends data to every other subscriber of channel.
func (c *RealtimeConn) Publish(channel string, data any) error {
	return c.send(map[string]any{"type": "publish", "channel": channel, "data": data})
}

// Direct sends data to one specific user across every device/replica
// they're connected from (server-side Redis-fanned pub/sub, Phase 10).
func (c *RealtimeConn) Direct(targetUserID string, data any) error {
	return c.send(map[string]any{"type": "direct", "targetUserId": targetUserID, "data": data})
}

func (c *RealtimeConn) send(msg map[string]any) error {
	return c.ws.WriteJSON(msg)
}

// Read blocks for the next server-sent message. Callers typically run
// this in a loop on its own goroutine.
func (c *RealtimeConn) Read() (RealtimeMessage, error) {
	var msg RealtimeMessage
	err := c.ws.ReadJSON(&msg)
	return msg, err
}

// SetReadDeadline is exposed directly so a caller can build their own
// keepalive/timeout strategy on top, rather than this SDK guessing one.
func (c *RealtimeConn) SetReadDeadline(t time.Time) error { return c.ws.SetReadDeadline(t) }

// Close closes the underlying connection.
func (c *RealtimeConn) Close() error { return c.ws.Close() }
