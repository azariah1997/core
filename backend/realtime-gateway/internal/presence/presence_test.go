package presence_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/example/core-platform/backend/realtime-gateway/internal/presence"
)

func newTracker(t *testing.T) (*presence.Tracker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return presence.New(client, 45*time.Second), mr
}

func TestConnectMakesUserOnline(t *testing.T) {
	tr, _ := newTracker(t)
	ctx := context.Background()

	online, err := tr.IsOnline(ctx, "user-1")
	if err != nil {
		t.Fatalf("is online (before): %v", err)
	}
	if online {
		t.Fatal("expected user to be offline before connecting")
	}

	if err := tr.Connect(ctx, "user-1", "device-1", "conn-1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	online, err = tr.IsOnline(ctx, "user-1")
	if err != nil {
		t.Fatalf("is online (after): %v", err)
	}
	if !online {
		t.Fatal("expected user to be online after connecting")
	}
}

func TestDevicesListsMultipleConnectedDevices(t *testing.T) {
	tr, _ := newTracker(t)
	ctx := context.Background()

	if err := tr.Connect(ctx, "user-1", "phone", "conn-1"); err != nil {
		t.Fatalf("connect phone: %v", err)
	}
	if err := tr.Connect(ctx, "user-1", "laptop", "conn-2"); err != nil {
		t.Fatalf("connect laptop: %v", err)
	}

	devices, err := tr.Devices(ctx, "user-1")
	if err != nil {
		t.Fatalf("devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %v", devices)
	}
}

func TestDisconnectRemovesPresence(t *testing.T) {
	tr, _ := newTracker(t)
	ctx := context.Background()

	if err := tr.Connect(ctx, "user-1", "device-1", "conn-1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := tr.Disconnect(ctx, "user-1", "device-1"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	online, err := tr.IsOnline(ctx, "user-1")
	if err != nil {
		t.Fatalf("is online: %v", err)
	}
	if online {
		t.Fatal("expected user to be offline after disconnect")
	}
}

func TestUncleanDisconnectExpiresViaTTL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	tr := presence.New(client, 1*time.Second)
	ctx := context.Background()

	if err := tr.Connect(ctx, "user-1", "device-1", "conn-1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Simulate time passing without a clean Disconnect (crash, network
	// partition) - miniredis supports fast-forwarding its internal clock
	// rather than a real sleep.
	mr.FastForward(2 * time.Second)

	online, err := tr.IsOnline(ctx, "user-1")
	if err != nil {
		t.Fatalf("is online: %v", err)
	}
	if online {
		t.Fatal("expected presence to have expired via TTL without an explicit disconnect")
	}
}

func TestHeartbeatRefreshesTTL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	tr := presence.New(client, 2*time.Second)
	ctx := context.Background()

	if err := tr.Connect(ctx, "user-1", "device-1", "conn-1"); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mr.FastForward(1 * time.Second)
	if err := tr.Heartbeat(ctx, "user-1", "device-1"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	mr.FastForward(1500 * time.Millisecond) // would have expired without the heartbeat refresh

	online, err := tr.IsOnline(ctx, "user-1")
	if err != nil {
		t.Fatalf("is online: %v", err)
	}
	if !online {
		t.Fatal("expected heartbeat to have kept presence alive past the original TTL")
	}
}
