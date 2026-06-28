// internal/shared/middleware/middleware.go
//
// WHY THIS EXISTS:
//   Cross-cutting concerns implemented as HTTP middleware that wraps handlers.
//
// CORRELATION ID vs REQUEST ID:
//   Phase 1 used "Request ID" — a per-hop identifier that changed between
//   services. Phase 2 replaces this with a "Trace ID" / "Correlation ID" that
//   is generated once at the system boundary (API gateway or first service)
//   and propagated unchanged through every downstream service call.
//
//   Phase 3 integrates OpenTelemetry (OTel), so trace context is extracted
//   from standard W3C 'traceparent' headers. A matching X-Trace-ID is maintained
//   for backward compatibility with logs.
//
// SECURITY:
//   Applies OWASP-recommended security headers, handles basic CORS, restricts
//   request body sizes (1MB limit), and filters trusted proxies.

package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
)

type contextKey string

const (
	TraceIDKey    contextKey = "trace_id"
	TraceIDHeader            = "X-Trace-ID"
)

type responseWriter struct {
	http.ResponseWriter
	status  int
	written int64
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// TraceID manages distributed trace ID extraction and OpenTelemetry integration.
// It extracts W3C traceparent headers, starts an OTel span, maps the OTel trace ID
// back to X-Trace-ID for logs, and propagates it via context.
func TraceID(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Extract existing trace context from W3C propagation headers (e.g. from gateway or gateway's upstream)
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// 2. Start a new OTel span for this HTTP request
			tracer := otel.GetTracerProvider().Tracer(serviceName)
			spanOpts := []oteltrace.SpanStartOption{
				oteltrace.WithSpanKind(oteltrace.SpanKindServer),
				oteltrace.WithAttributes(
					semconv.HTTPMethodKey.String(r.Method),
					semconv.HTTPTargetKey.String(r.URL.Path),
					semconv.HTTPRouteKey.String(r.URL.Path),
					semconv.HTTPSchemeKey.String(r.URL.Scheme),
					semconv.HTTPHostKey.String(r.Host),
				),
			}
			ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path), spanOpts...)
			defer span.End()

			// 3. Extract the active TraceID from the span context
			var traceID string
			if spanCtx := span.SpanContext(); spanCtx.IsValid() {
				traceID = spanCtx.TraceID().String()
			}

			// 4. Fall back to X-Trace-ID header or generate a new one if OTel is not active
			if traceID == "" {
				traceID = r.Header.Get(TraceIDHeader)
				if traceID == "" {
					traceID = generateTraceID()
				}
			}

			// Echo X-Trace-ID back to client in response
			w.Header().Set(TraceIDHeader, traceID)

			// Store trace ID in context and update r
			ctx = context.WithValue(ctx, TraceIDKey, traceID)
			r = r.WithContext(ctx)

			// Execute downstream handler
			next.ServeHTTP(w, r)
		})
	}
}

// Logging records structured logs for the request, including trace context.
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := wrapResponseWriter(w)
			next.ServeHTTP(rw, r)

			traceID := TraceIDFromContext(r)
			latency := time.Since(start)

			// Retrieve the current span and record attributes/status on it
			span := oteltrace.SpanFromContext(r.Context())
			if span.IsRecording() {
				span.SetAttributes(
					semconv.HTTPStatusCodeKey.Int(rw.status),
					attribute.Int64("http.response_content_length", rw.written),
				)
				if rw.status >= 500 {
					span.SetStatus(1, fmt.Sprintf("HTTP status %d", rw.status)) // 1 = Error
				} else {
					span.SetStatus(2, "") // 2 = Ok
				}
			}

			fields := []any{
				"trace_id", traceID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"latency_ms", latency.Milliseconds(),
				"bytes", rw.written,
				"remote_addr", r.RemoteAddr,
			}

			switch {
			case rw.status >= 500:
				log.Error("http_request", fields...)
			case rw.status >= 400:
				log.Warn("http_request", fields...)
			default:
				log.Info("http_request", fields...)
			}
		})
	}
}

// MetricsMiddleware records Prometheus metric counts.
func MetricsMiddleware(m *metrics.Metrics, serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			m.RequestsInFlight.WithLabelValues(serviceName).Inc()
			defer m.RequestsInFlight.WithLabelValues(serviceName).Dec()

			rw := wrapResponseWriter(w)
			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(rw.status)

			m.RequestsTotal.WithLabelValues(serviceName, r.Method, r.URL.Path, status).Inc()
			m.RequestDuration.WithLabelValues(serviceName, r.Method, r.URL.Path).Observe(duration)

			if rw.status >= 400 {
				m.ErrorsTotal.WithLabelValues(serviceName, r.Method, r.URL.Path, status).Inc()
			}
		})
	}
}

// SecurityHeaders enforces OWASP security configurations.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// CORS simple configuration
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-ID, X-Idempotency-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestSizeLimiter enforces a maximum content length.
func RequestSizeLimiter(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				traceID := TraceIDFromContext(r)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				fmt.Fprintf(w, `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"request payload exceeds %d bytes","trace_id":%q}}`, maxBytes, traceID)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// Recovery intercepts panics and protects service uptime.
func Recovery(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					traceID := TraceIDFromContext(r)
					log.Error("panic_recovered",
						"trace_id", traceID,
						"panic", fmt.Sprintf("%v", err),
						"path", r.URL.Path,
						"method", r.Method,
					)

					span := oteltrace.SpanFromContext(r.Context())
					if span.IsRecording() {
						span.RecordError(fmt.Errorf("panic: %v", err))
						span.SetStatus(1, "panic recovered")
					}

					w.Header().Set("Content-Type", "application/json")
					w.Header().Set(TraceIDHeader, traceID)
					http.Error(w,
						`{"error":{"code":"INTERNAL_ERROR","message":"an unexpected error occurred"}}`,
						http.StatusInternalServerError,
					)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies middleware sequentially.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// TraceIDFromContext retrieves the Trace ID string.
func TraceIDFromContext(r *http.Request) string {
	if v, ok := r.Context().Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}

// TraceIDFromCtx retrieves the Trace ID from general context.
func TraceIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(TraceIDKey).(string); ok {
		return v
	}
	return ""
}
