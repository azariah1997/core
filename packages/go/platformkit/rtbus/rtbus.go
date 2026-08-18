// Package rtbus is the shared Redis pub/sub wire contract for realtime fan-out
// across gateway replicas. It exists so any service that needs to push a
// realtime event - not just realtime-gateway's own hub - speaks the exact
// same envelope, rather than each caller hand-rolling a compatible-by-luck
// JSON shape that eventually drifts.
package rtbus

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

const (
	ChannelPrefix = "rt:channel:"
	UserPrefix    = "rt:user:"
	DevicePrefix  = "rt:device:"
)

// Kind identifies how a Message should be routed once it reaches a gateway
// replica: to everyone subscribed to a channel, to every device of a user,
// or to one specific device.
type Kind string

const (
	KindChannel Kind = "channel"
	KindUser    Kind = "user"
	KindDevice  Kind = "device"
)

// Message is the envelope published on Redis. Payload is opaque to rtbus -
// it's whatever the caller wants delivered verbatim to the connection.
type Message struct {
	Kind    Kind            `json:"kind"`
	Channel string          `json:"channel,omitempty"`
	Target  string          `json:"target,omitempty"` // userID, or userID+"|"+deviceID
	Payload json.RawMessage `json:"payload"`
}

// Publisher writes Messages to Redis. It has no knowledge of who's
// subscribed - that's realtime-gateway's hub package, running in a
// different process (or replica) than most Publisher callers.
type Publisher struct {
	redis *redis.Client
}

func NewPublisher(client *redis.Client) *Publisher {
	return &Publisher{redis: client}
}

func (p *Publisher) ToChannel(ctx context.Context, channel string, payload json.RawMessage) error {
	return p.publish(ctx, ChannelPrefix+channel, Message{Kind: KindChannel, Channel: channel, Payload: payload})
}

func (p *Publisher) ToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	return p.publish(ctx, UserPrefix+userID, Message{Kind: KindUser, Target: userID, Payload: payload})
}

func (p *Publisher) ToDevice(ctx context.Context, userID, deviceID string, payload json.RawMessage) error {
	target := DeviceTarget(userID, deviceID)
	return p.publish(ctx, DevicePrefix+target, Message{Kind: KindDevice, Target: target, Payload: payload})
}

func (p *Publisher) publish(ctx context.Context, redisChannel string, m Message) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return p.redis.Publish(ctx, redisChannel, body).Err()
}

// DeviceTarget and SplitDeviceTarget are the shared encoding for a
// (userID, deviceID) pair used as a device Message's Target.
func DeviceTarget(userID, deviceID string) string {
	return userID + "|" + deviceID
}

func SplitDeviceTarget(target string) (userID, deviceID string, ok bool) {
	for i := 0; i < len(target); i++ {
		if target[i] == '|' {
			return target[:i], target[i+1:], true
		}
	}
	return "", "", false
}
