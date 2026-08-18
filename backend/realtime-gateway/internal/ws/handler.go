// Package ws is the authenticated WebSocket connection handler: upgrade,
// heartbeat, and the client message protocol (subscribe/unsubscribe/
// publish/direct) built on top of hub.Hub and presence.Tracker.
package ws

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/example/core-platform/backend/realtime-gateway/internal/hub"
	"github.com/example/core-platform/backend/realtime-gateway/internal/presence"
)

const wsMagic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	// pingInterval is how often the server pings an idle connection.
	pingInterval = 25 * time.Second
	// readTimeout must comfortably exceed pingInterval so a client that's
	// still alive (even if it only just missed responding to the last
	// ping) isn't dropped prematurely.
	readTimeout = 70 * time.Second
)

type Handler struct {
	hub      *hub.Hub
	presence *presence.Tracker
	logger   *slog.Logger
}

func NewHandler(h *hub.Hub, p *presence.Tracker, logger *slog.Logger) *Handler {
	return &Handler{hub: h, presence: p, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth, ok := AuthFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	netConn, buf, err := hj.Hijack()
	if err != nil {
		return
	}

	sum := sha1.Sum([]byte(key + wsMagic))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err := buf.Flush(); err != nil {
		_ = netConn.Close()
		return
	}

	h.serve(r.Context(), netConn, auth)
}

// conn bundles the hub handle with a dedicated channel for control-frame
// replies (pongs), so every write to netConn - data, pings, pong replies -
// funnels through the single writeLoop goroutine. Writing from more than
// one goroutine would interleave frames on the wire and corrupt the
// stream; net.Conn itself only guarantees safety for one concurrent
// reader and one concurrent writer, not two writers.
type conn struct {
	*hub.Conn
	pong chan []byte
}

func (h *Handler) serve(ctx context.Context, netConn net.Conn, auth Auth) {
	defer func() { _ = netConn.Close() }()

	c := &conn{
		Conn: &hub.Conn{ID: uuid.NewString(), UserID: auth.UserID, DeviceID: auth.DeviceID, Send: make(chan []byte, 32)},
		pong: make(chan []byte, 4),
	}

	h.hub.Register(c.Conn)
	if err := h.presence.Connect(ctx, auth.UserID, auth.DeviceID, c.ID); err != nil {
		h.logger.Error("presence connect failed", "error", err, "userId", auth.UserID)
	}
	defer func() {
		h.hub.Unregister(c.Conn)
		dctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := h.presence.Disconnect(dctx, auth.UserID, auth.DeviceID); err != nil {
			h.logger.Error("presence disconnect failed", "error", err, "userId", auth.UserID)
		}
	}()

	done := make(chan struct{})
	go h.writeLoop(netConn, c, done)
	defer close(done)

	h.sendServer(c, serverMessage{Type: "connected", ConnectionID: c.ID, UserID: auth.UserID, DeviceID: auth.DeviceID})
	h.readLoop(ctx, netConn, c)
}

func (h *Handler) writeLoop(netConn net.Conn, c *conn, done <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case payload, ok := <-c.Send:
			if !ok {
				// hub.Register closed Send to evict this connection (a
				// reconnect on the same user+device beat us to it). Close
				// the socket so readLoop's blocked Read unblocks
				// immediately instead of lingering until readTimeout.
				_ = netConn.Close()
				return
			}
			if err := writeFrame(netConn, 1, payload); err != nil {
				return
			}
		case payload := <-c.pong:
			if err := writeFrame(netConn, 10, payload); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeFrame(netConn, 9, nil); err != nil {
				return
			}
		}
	}
}

func (h *Handler) readLoop(ctx context.Context, netConn net.Conn, c *conn) {
	for {
		_ = netConn.SetReadDeadline(time.Now().Add(readTimeout))
		opcode, payload, err := readFrame(netConn)
		if err != nil {
			return
		}

		switch opcode {
		case 8: // close
			return
		case 9: // client ping -> reply pong via the single writer
			select {
			case c.pong <- payload:
			default:
			}
		case 10: // client pong: nothing to do beyond the read deadline already resetting
		case 1: // text
			h.handleClientMessage(ctx, c, payload)
		}

		if err := h.presence.Heartbeat(ctx, c.UserID, c.DeviceID); err != nil {
			h.logger.Error("presence heartbeat failed", "error", err, "userId", c.UserID)
		}
	}
}

func (h *Handler) handleClientMessage(ctx context.Context, c *conn, raw []byte) {
	var msg clientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		h.sendServer(c, serverMessage{Type: "error", Message: "invalid JSON message"})
		return
	}

	switch msg.Type {
	case "subscribe":
		if msg.Channel == "" {
			h.sendServer(c, serverMessage{Type: "error", Message: "channel is required"})
			return
		}
		h.hub.Subscribe(c.ID, msg.Channel)
		h.sendServer(c, serverMessage{Type: "subscribed", Channel: msg.Channel})

	case "unsubscribe":
		h.hub.Unsubscribe(c.ID, msg.Channel)
		h.sendServer(c, serverMessage{Type: "unsubscribed", Channel: msg.Channel})

	case "publish":
		if msg.Channel == "" {
			h.sendServer(c, serverMessage{Type: "error", Message: "channel is required"})
			return
		}
		if !h.hub.IsSubscribed(c.ID, msg.Channel) {
			h.sendServer(c, serverMessage{Type: "error", Message: "must subscribe before publishing to a channel"})
			return
		}
		payload, err := json.Marshal(serverMessage{Type: "message", Channel: msg.Channel, FromUserID: c.UserID, Data: msg.Data})
		if err != nil {
			return
		}
		if err := h.hub.PublishToChannel(ctx, msg.Channel, payload); err != nil {
			h.logger.Error("publish to channel failed", "error", err, "channel", msg.Channel)
		}

	case "direct":
		if msg.TargetUserID == "" {
			h.sendServer(c, serverMessage{Type: "error", Message: "targetUserId is required"})
			return
		}
		payload, err := json.Marshal(serverMessage{Type: "direct", FromUserID: c.UserID, Data: msg.Data})
		if err != nil {
			return
		}
		if err := h.hub.PublishToUser(ctx, msg.TargetUserID, payload); err != nil {
			h.logger.Error("publish direct event failed", "error", err, "targetUserId", msg.TargetUserID)
		}

	default:
		h.sendServer(c, serverMessage{Type: "error", Message: "unknown message type"})
	}
}

func (h *Handler) sendServer(c *conn, m serverMessage) {
	body, err := json.Marshal(m)
	if err != nil {
		return
	}
	select {
	case c.Send <- body:
	default:
		h.logger.Error("dropped outbound message: send buffer full", "connectionId", c.ID)
	}
}
