// Package pulsemodules holds pulseprefs' adapter onto sibling Pulse
// modules - consumed in-process, same pattern established by mood/bond
// (Phase 8), livetouch/bond (Phase 10), and signals|moments/
// pulseinteractions (Phase 11-12).
package pulsemodules

import (
	"context"

	"github.com/example/core-platform/apps/pulse/api/internal/pulseprefs"
)

// Notifier is the shape both pulseinteractions.Notifier and
// livetouch.Notifier already share (this codebase's
// consumer-defined-interface convention) - one decorator type
// structurally satisfies both without needing two separate adapters.
type Notifier interface {
	Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error
}

// NotifierDecorator wraps a real Notifier and consults the receiver's
// own Pulse-owned notification-detail preference (spec §72) - never
// the sender's, this is entirely the receiver's own setting - before
// calling through. Reading pulseprefs.Service.Get by userID here needs
// no per-caller Core client and crosses no auth boundary: this is
// Pulse's own database, read by Pulse's own backend code, the same way
// moments.Service already freely resolves "the other participant" -
// the boundary that matters is who's *allowed to see* the data, not
// who's allowed to read the row, and only Pulse's own server code ever
// sees this decision.
type NotifierDecorator struct {
	inner Notifier
	prefs *pulseprefs.Service
}

func NewNotifierDecorator(inner Notifier, prefs *pulseprefs.Service) NotifierDecorator {
	return NotifierDecorator{inner: inner, prefs: prefs}
}

func (d NotifierDecorator) Notify(ctx context.Context, receiverUserID, category, title, body string, data map[string]any) error {
	p, err := d.prefs.Get(ctx, receiverUserID)
	if err != nil {
		// A preferences lookup failure should never block a real
		// notification - fail open to the pre-Phase-13 behavior rather
		// than silently losing delivery.
		return d.inner.Notify(ctx, receiverUserID, category, title, body, data)
	}
	switch p.NotificationDetail {
	case pulseprefs.SilentNotification:
		return nil
	case pulseprefs.PrivateNotification:
		return d.inner.Notify(ctx, receiverUserID, category, "Pulse", "You have new activity.", nil)
	default:
		return d.inner.Notify(ctx, receiverUserID, category, title, body, data)
	}
}
