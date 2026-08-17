// Package otelx wires each service into the platform's shared OpenTelemetry
// collector so requests are observable as traces, not just logs.
package otelx

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	ServiceName string
	Environment string
	// Endpoint is the OTLP/HTTP collector address, e.g. "http://localhost:4318".
	// Tracing is disabled when empty.
	Endpoint string
}

// Init configures the global trace provider and text map propagator. The
// returned shutdown func flushes and closes the exporter; callers must defer
// it. Exporter connectivity problems are handled asynchronously by the
// OTel SDK's batch processor and never fail a request.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if cfg.Endpoint == "" {
		return noop, nil
	}

	host, insecure := cfg.Endpoint, true
	if u, err := url.Parse(cfg.Endpoint); err == nil && u.Host != "" {
		host = u.Host
		insecure = u.Scheme != "https"
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(host)}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return noop, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return noop, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return func(shutdownCtx context.Context) error {
		c, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()
		return tp.Shutdown(c)
	}, nil
}

// Wrap instruments an HTTP handler so every request produces a span and
// propagates trace context from inbound headers.
func Wrap(serviceName string, next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, serviceName)
}
