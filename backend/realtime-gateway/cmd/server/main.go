package main

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/example/core-platform/packages/go/platformkit/config"
	"github.com/example/core-platform/packages/go/platformkit/correlation"
	"github.com/example/core-platform/packages/go/platformkit/health"
	"github.com/example/core-platform/packages/go/platformkit/logging"
	"github.com/example/core-platform/packages/go/platformkit/otelx"
	"github.com/example/core-platform/packages/go/platformkit/runx"
)

const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
const serviceName = "realtime-gateway"

func main() {
	cfg := config.Load()
	logger := logging.New(serviceName, cfg.Env)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	shutdownTracing, err := otelx.Init(ctx, otelx.Config{
		ServiceName: serviceName,
		Environment: cfg.Env,
		Endpoint:    cfg.OtelEndpoint,
	})
	if err != nil {
		logger.Error("failed to initialise tracing", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	mux := http.NewServeMux()
	// The realtime gateway has no external dependency it must gate
	// traffic on today; readiness tracks liveness until presence/session
	// stores are wired in Phase 10.
	mux.HandleFunc("GET /livez", health.Live(serviceName))
	mux.HandleFunc("GET /readyz", health.Ready(nil))
	mux.HandleFunc("GET /healthz", health.Health(serviceName, nil))
	mux.HandleFunc("GET /ws", ws)

	handler := otelx.Wrap(serviceName, correlation.Middleware(mux))
	srv := &http.Server{Addr: cfg.RealtimeAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	if err := runx.Serve(ctx, logger, srv); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func ws(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket upgrade required", 426)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing key", 400)
		return
	}
	h := sha1.Sum([]byte(key + magic))
	accept := base64.StdEncoding.EncodeToString(h[:])
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", 500)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	_ = buf.Flush()
	_ = writeText(conn, `{"type":"platform.connected","version":1}`)
	for {
		opcode, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		if opcode == 8 {
			return
		}
		if opcode == 9 {
			_ = writeFrame(conn, 10, payload)
			continue
		}
		if opcode == 1 {
			_ = writeText(conn, string(payload))
		}
	}
}

func readFrame(r io.Reader) (byte, []byte, error) {
	h := make([]byte, 2)
	if _, e := io.ReadFull(r, h); e != nil {
		return 0, nil, e
	}
	op := h[0] & 0x0f
	masked := h[1]&0x80 != 0
	n := int(h[1] & 0x7f)
	if n == 126 {
		b := make([]byte, 2)
		if _, e := io.ReadFull(r, b); e != nil {
			return 0, nil, e
		}
		n = int(binary.BigEndian.Uint16(b))
	}
	if n == 127 {
		b := make([]byte, 8)
		if _, e := io.ReadFull(r, b); e != nil {
			return 0, nil, e
		}
		n = int(binary.BigEndian.Uint64(b))
	}
	mask := make([]byte, 4)
	if masked {
		if _, e := io.ReadFull(r, mask); e != nil {
			return 0, nil, e
		}
	}
	p := make([]byte, n)
	if _, e := io.ReadFull(r, p); e != nil {
		return 0, nil, e
	}
	if masked {
		for i := range p {
			p[i] ^= mask[i%4]
		}
	}
	return op, p, nil
}
func writeText(w io.Writer, s string) error { return writeFrame(w, 1, []byte(s)) }
func writeFrame(w io.Writer, op byte, p []byte) error {
	h := []byte{0x80 | op}
	n := len(p)
	if n < 126 {
		h = append(h, byte(n))
	} else if n <= 65535 {
		h = append(h, 126, byte(n>>8), byte(n))
	} else {
		h = append(h, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	if _, e := w.Write(h); e != nil {
		return e
	}
	_, e := w.Write(p)
	return e
}
