package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// NewWithLoki builds the same structured stdout logger New does, plus a
// second, real destination: every record is also shipped to Loki's own
// native push API (POST /loki/api/v1/push - no separate log-shipping
// agent needed, unlike a Promtail-based setup, which also wouldn't see
// these services' logs anyway since they run as bare `go run` host
// processes, not inside a container Promtail could tail). When a
// record is logged with a context carrying an active OTel span (via
// *Context slog methods - InfoContext, ErrorContext, etc.), its real
// trace_id/span_id are attached, which is what lets Grafana's Loki
// datasource jump straight from a log line to its trace (see
// infra/observability/grafana/provisioning/datasources's derivedFields).
//
// Shipping is fire-and-forget with a short timeout - a Loki outage or
// slow network must never slow down or fail the request that's being
// logged.
func NewWithLoki(service, env, lokiPushURL string) *slog.Logger {
	handler := &lokiHandler{
		next:    newBaseHandler(),
		service: service,
		env:     env,
		pushURL: lokiPushURL,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
	return slog.New(handler).With("service", service, "env", env)
}

type lokiHandler struct {
	next    slog.Handler
	service string
	env     string
	pushURL string
	client  *http.Client
	attrs   []slog.Attr
}

func (h *lokiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *lokiHandler) Handle(ctx context.Context, r slog.Record) error {
	go h.push(ctx, r)
	return h.next.Handle(ctx, r)
}

func (h *lokiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &lokiHandler{
		next: h.next.WithAttrs(attrs), service: h.service, env: h.env,
		pushURL: h.pushURL, client: h.client, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

func (h *lokiHandler) WithGroup(name string) slog.Handler {
	return &lokiHandler{
		next: h.next.WithGroup(name), service: h.service, env: h.env,
		pushURL: h.pushURL, client: h.client, attrs: h.attrs,
	}
}

func (h *lokiHandler) push(ctx context.Context, r slog.Record) {
	line := map[string]any{
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	for _, a := range h.attrs {
		line[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		line[a.Key] = a.Value.Any()
		return true
	})
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		line["traceId"] = span.TraceID().String()
		line["spanId"] = span.SpanID().String()
	}
	body, err := json.Marshal(line)
	if err != nil {
		return
	}

	payload := lokiPushRequest{
		Streams: []lokiStream{{
			Stream: map[string]string{"service": h.service, "env": h.env, "level": r.Level.String()},
			Values: [][2]string{{strconv.FormatInt(r.Time.UnixNano(), 10), string(body)}},
		}},
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return
	}

	pushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pushCtx, http.MethodPost, h.pushURL, bytes.NewReader(reqBody))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}
