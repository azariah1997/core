// Package runx runs an HTTP server with signal-driven graceful shutdown so
// in-flight requests can drain before the process exits.
package runx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// ShutdownTimeout bounds how long Serve waits for in-flight requests to
// drain after a shutdown signal before forcing the listener closed.
const ShutdownTimeout = 10 * time.Second

// Serve starts srv and blocks until it fails or ctx receives SIGINT/SIGTERM,
// at which point it gracefully shuts srv down.
func Serve(ctx context.Context, logger *slog.Logger, srv *http.Server) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}
