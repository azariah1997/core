// Package logging provides the platform's structured logger.
package logging

import (
	"log/slog"
	"os"
)

// New returns a JSON structured logger tagged with the owning service and
// environment, matching every downstream log line without callers needing
// to repeat those fields.
func New(service, env string) *slog.Logger {
	return slog.New(newBaseHandler()).With("service", service, "env", env)
}

func newBaseHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
}
