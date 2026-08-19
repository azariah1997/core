// Package hub manages this process's local WebSocket connections and
// bridges channel/direct-user/device messages across gateway replicas via
// Redis pub/sub - a message published on one instance reaches subscribers
// connected to any instance, which is why the Helm chart can safely run
// realtime-gateway at replicas: 2. Nothing here is persisted; a message
// published while a target is offline is simply not delivered - realtime
// delivery, not a durable outbox (that's Messaging, Phase 11).
package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/packages/go/platformkit/metrics"
	"github.com/example/core-platform/packages/go/platformkit/rtbus"
)

// Conn is a local WebSocket connection. Send is the outbound queue the
// connection's write loop drains; Hub never writes to the socket directly.
type Conn struct {
	ID       string
	UserID   string
	DeviceID string
	Send     chan []byte
}

type Hub struct {
	redis     *redis.Client
	publisher *rtbus.Publisher
	logger    *slog.Logger

	mu     sync.RWMutex
	conns  map[string]*Conn            // connID -> conn
	subs   map[string]map[string]bool  // channel -> set of connIDs
	byUser map[string]map[string]*Conn // userID -> deviceID -> conn
}

func New(redisClient *redis.Client, logger *slog.Logger) *Hub {
	return &Hub{
		redis:     redisClient,
		publisher: rtbus.NewPublisher(redisClient),
		logger:    logger,
		conns:     map[string]*Conn{},
		subs:      map[string]map[string]bool{},
		byUser:    map[string]map[string]*Conn{},
	}
}

// Run subscribes to the platform-wide pub/sub pattern and fans incoming
// messages out to matching local connections until ctx is cancelled. Any
// service holding an rtbus.Publisher on the same Redis - not just this
// hub's own PublishTo* methods - can trigger delivery here.
func (h *Hub) Run(ctx context.Context) {
	pubsub := h.redis.PSubscribe(ctx, rtbus.ChannelPrefix+"*", rtbus.UserPrefix+"*", rtbus.DevicePrefix+"*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			h.dispatch(msg.Payload)
		}
	}
}

func (h *Hub) dispatch(raw string) {
	var m rtbus.Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		h.logger.Error("hub: malformed pub/sub message", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	switch m.Kind {
	case rtbus.KindChannel:
		for connID := range h.subs[m.Channel] {
			if c, ok := h.conns[connID]; ok {
				nonBlockingSend(c, m.Payload)
			}
		}
	case rtbus.KindUser:
		for _, c := range h.byUser[m.Target] {
			nonBlockingSend(c, m.Payload)
		}
	case rtbus.KindDevice:
		userID, deviceID, _ := rtbus.SplitDeviceTarget(m.Target)
		if devices, ok := h.byUser[userID]; ok {
			if c, ok := devices[deviceID]; ok {
				nonBlockingSend(c, m.Payload)
			}
		}
	}
}

func nonBlockingSend(c *Conn, payload []byte) {
	select {
	case c.Send <- payload:
	default:
		// Slow consumer: drop rather than block the hub for everyone else.
	}
}

// Register adds a connection, evicting any existing local connection for
// the same (userID, deviceID) - the reconnect case on this instance.
// Cross-instance eviction (the old connection lives on a different
// replica) isn't handled: that needs a dedicated "kick" broadcast this
// package doesn't implement yet.
func (h *Hub) Register(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if devices, ok := h.byUser[c.UserID]; ok {
		if old, ok := devices[c.DeviceID]; ok {
			close(old.Send)
			delete(h.conns, old.ID)
			// A same-user-same-device reconnect replaces one conns entry
			// with another - net zero for the gauge, so this decrement is
			// paired with the unconditional increment below, not a bug.
			metrics.DecRealtimeConnections()
		}
	} else {
		h.byUser[c.UserID] = map[string]*Conn{}
	}

	h.conns[c.ID] = c
	h.byUser[c.UserID][c.DeviceID] = c
	metrics.IncRealtimeConnections()
}

// Unregister removes a connection and all its channel subscriptions.
func (h *Hub) Unregister(c *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// A connection Register already replaced (see above) was removed
	// from conns there, not here - guard against double-decrementing
	// the gauge if its own goroutine still calls Unregister afterward.
	if _, stillPresent := h.conns[c.ID]; stillPresent {
		metrics.DecRealtimeConnections()
	}
	delete(h.conns, c.ID)
	if devices, ok := h.byUser[c.UserID]; ok {
		if devices[c.DeviceID] == c {
			delete(devices, c.DeviceID)
		}
		if len(devices) == 0 {
			delete(h.byUser, c.UserID)
		}
	}
	for channel, members := range h.subs {
		delete(members, c.ID)
		if len(members) == 0 {
			delete(h.subs, channel)
		}
	}
}

func (h *Hub) Subscribe(connID, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[channel] == nil {
		h.subs[channel] = map[string]bool{}
	}
	h.subs[channel][connID] = true
}

func (h *Hub) Unsubscribe(connID, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[channel], connID)
}

// IsSubscribed reports whether connID has joined channel - publishing is
// restricted to channels the caller has already joined.
func (h *Hub) IsSubscribed(connID, channel string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.subs[channel][connID]
}

// PublishToChannel fans payload out to every subscriber of channel on
// every gateway replica.
func (h *Hub) PublishToChannel(ctx context.Context, channel string, payload json.RawMessage) error {
	return h.publisher.ToChannel(ctx, channel, payload)
}

// PublishToUser delivers payload to every device userID is connected on,
// across every gateway replica.
func (h *Hub) PublishToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	return h.publisher.ToUser(ctx, userID, payload)
}

// PublishToDevice delivers payload to exactly one (userID, deviceID)
// connection, wherever it's connected.
func (h *Hub) PublishToDevice(ctx context.Context, userID, deviceID string, payload json.RawMessage) error {
	return h.publisher.ToDevice(ctx, userID, deviceID, payload)
}
