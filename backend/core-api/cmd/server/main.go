package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/example/core-platform/backend/core-api/internal/api"
	"github.com/example/core-platform/packages/go/platformkit/config"
)

func main() {
	cfg := config.Load()
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: api.New(cfg), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("core-api listening on %s env=%s", cfg.HTTPAddr, cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	_ = srv.Shutdown(context.Background())
}
