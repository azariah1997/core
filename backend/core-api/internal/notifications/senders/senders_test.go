package senders_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/example/core-platform/backend/core-api/internal/notifications"
	"github.com/example/core-platform/backend/core-api/internal/notifications/senders"
)

type fakeDeviceLookup struct{ hasToken bool }

func (f fakeDeviceLookup) HasPushToken(ctx context.Context, userID string) (bool, error) {
	return f.hasToken, nil
}

func TestPushSenderFailsWithoutARegisteredPushToken(t *testing.T) {
	s := senders.PushSender{Devices: fakeDeviceLookup{hasToken: false}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, err := s.Send(context.Background(), notifications.Notification{ID: "n1", UserID: "u1"})
	if err == nil {
		t.Fatal("expected an error when the recipient has no registered push token")
	}
}

func TestPushSenderSucceedsWithARegisteredPushToken(t *testing.T) {
	s := senders.PushSender{Devices: fakeDeviceLookup{hasToken: true}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ref, err := s.Send(context.Background(), notifications.Notification{ID: "n1", UserID: "u1"})
	if err != nil {
		t.Fatalf("expected success with a registered push token, got %v", err)
	}
	if ref == "" {
		t.Fatal("expected a non-empty provider reference")
	}
}

type fakeRealtime struct {
	userID  string
	payload json.RawMessage
}

func (f *fakeRealtime) ToUser(ctx context.Context, userID string, payload json.RawMessage) error {
	f.userID = userID
	f.payload = payload
	return nil
}

func TestRealtimeSenderPublishesToTheRecipient(t *testing.T) {
	rt := &fakeRealtime{}
	s := senders.RealtimeSender{Realtime: rt}
	n := notifications.Notification{ID: "n1", UserID: "u1", Category: "message", Title: "Hi", Body: "there"}
	if _, err := s.Send(context.Background(), n); err != nil {
		t.Fatalf("send: %v", err)
	}
	if rt.userID != "u1" {
		t.Fatalf("expected publish to target u1, got %q", rt.userID)
	}
	var decoded map[string]any
	if err := json.Unmarshal(rt.payload, &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded["type"] != "notification" || decoded["title"] != "Hi" {
		t.Fatalf("unexpected payload: %v", decoded)
	}
}

func TestLogSenderAlwaysSucceeds(t *testing.T) {
	s := senders.LogSender{Channel: notifications.ChannelEmail, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ref, err := s.Send(context.Background(), notifications.Notification{ID: "n1", UserID: "u1"})
	if err != nil || ref == "" {
		t.Fatalf("expected LogSender to always succeed, got ref=%q err=%v", ref, err)
	}
}

func TestInAppSenderAlwaysSucceeds(t *testing.T) {
	s := senders.InAppSender{}
	ref, err := s.Send(context.Background(), notifications.Notification{ID: "n1", UserID: "u1"})
	if err != nil || ref == "" {
		t.Fatalf("expected InAppSender to always succeed, got ref=%q err=%v", ref, err)
	}
}
