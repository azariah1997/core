// Package presence tracks who is currently connected, backed entirely by
// Valkey/Redis with key expiry - never Postgres. A connection that dies
// without a clean close (crash, network partition) simply ages out of
// presence within TTL; there is no "critical realtime data" that needs to
// survive a restart or that a heartbeat-per-row table would justify.
package presence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "presence:"

type Tracker struct {
	client *redis.Client
	ttl    time.Duration
}

func New(client *redis.Client, ttl time.Duration) *Tracker {
	return &Tracker{client: client, ttl: ttl}
}

func key(userID, deviceID string) string {
	return keyPrefix + userID + ":" + deviceID
}

// Connect records that userID/deviceID has an active connection with the
// given connection ID, expiring after ttl unless refreshed by Heartbeat.
func (t *Tracker) Connect(ctx context.Context, userID, deviceID, connID string) error {
	if err := t.client.Set(ctx, key(userID, deviceID), connID, t.ttl).Err(); err != nil {
		return fmt.Errorf("presence connect: %w", err)
	}
	return nil
}

// Heartbeat refreshes the TTL for an already-connected device.
func (t *Tracker) Heartbeat(ctx context.Context, userID, deviceID string) error {
	if err := t.client.Expire(ctx, key(userID, deviceID), t.ttl).Err(); err != nil {
		return fmt.Errorf("presence heartbeat: %w", err)
	}
	return nil
}

// Disconnect is a best-effort explicit cleanup on clean close; TTL expiry
// is the backstop for unclean disconnects.
func (t *Tracker) Disconnect(ctx context.Context, userID, deviceID string) error {
	if err := t.client.Del(ctx, key(userID, deviceID)).Err(); err != nil {
		return fmt.Errorf("presence disconnect: %w", err)
	}
	return nil
}

// IsOnline reports whether userID has any active connection on any device.
func (t *Tracker) IsOnline(ctx context.Context, userID string) (bool, error) {
	devices, err := t.Devices(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(devices) > 0, nil
}

// Devices lists the device IDs userID currently has an active connection
// on. Uses SCAN, not KEYS, so it never blocks Redis even with many keys.
func (t *Tracker) Devices(ctx context.Context, userID string) ([]string, error) {
	pattern := keyPrefix + userID + ":*"
	var devices []string
	iter := t.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		devices = append(devices, strings.TrimPrefix(iter.Val(), keyPrefix+userID+":"))
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("presence list devices: %w", err)
	}
	return devices, nil
}
