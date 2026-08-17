package main

import (
	"github.com/example/core-platform/packages/go/platformkit/config"
	"log"
	"net/http"
	"time"
)

func main() {
	cfg := config.Load()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})
	go func() {
		for range time.Tick(30 * time.Second) {
			log.Printf("worker heartbeat brokers=%v", cfg.KafkaBrokers)
		}
	}()
	log.Printf("worker health endpoint listening on %s", cfg.WorkerAddr)
	log.Fatal(http.ListenAndServe(cfg.WorkerAddr, mux))
}
