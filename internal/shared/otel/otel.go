// internal/shared/otel/otel.go
//
// WHY THIS EXISTS:
//   Every microservice needs to initialize its OpenTelemetry tracer provider
//   to export trace spans to collectors like Jaeger or Tempo. Centralising
//   this setup ensures that all services use the same resource attributes,
//   exporters, and sampling strategies.
//
// DESIGN:
//   InitTracer initializes the W3C trace propagation standard, sets up the
//   OTLP trace exporter over HTTP, registers a batch span processor, and
//   returns a shutdown function to flush spans before the service exits.

package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// InitTracer configures global tracer provider and propagator.
// It returns a shutdown function that flushes pending spans during graceful shutdown.
func InitTracer(ctx context.Context, serviceName, version, endpoint string) (func(context.Context) error, error) {
	// If endpoint is empty, default to standard local OTel collector
	if endpoint == "" {
		endpoint = "localhost:4318" // OTLP HTTP receiver port
	}

	// 1. Create the OTLP Trace Exporter
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(), // In production, use TLS certificates
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// 2. Set up resource description
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(version),
			semconv.DeploymentEnvironmentKey.String("production"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	// 3. Set up the Tracer Provider with Batcher
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // In high traffic, use ratio sampler (e.g. 0.1)
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	// Set up the standard W3C text map propagator for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return a cleanup function
	shutdown := func(shutdownCtx context.Context) error {
		return tp.Shutdown(shutdownCtx)
	}

	return shutdown, nil
}
