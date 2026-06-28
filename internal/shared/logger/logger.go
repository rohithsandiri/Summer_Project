// internal/shared/logger/logger.go
//
// WHY THIS EXISTS:
//   Plain log.Printf produces unstructured text that is difficult to parse in
//   centralised log aggregators (Grafana Loki, ELK). Structured JSON logs let
//   Loki/Elasticsearch index every field and let Grafana build dashboards on
//   request_id, service, status_code, latency, etc.
//
// DESIGN:
//   We use the standard library's log/slog (Go 1.21+) which emits structured
//   key-value pairs. Each log record carries:
//     - time       (RFC3339 nanosecond)
//     - level      (DEBUG / INFO / WARN / ERROR)
//     - service    (from config)
//     - msg        (human-readable summary)
//     + any extra attributes (request_id, method, path, status, latency_ms)
//
// FUTURE PHASES:
//   - Add OpenTelemetry log bridge (slog → OTLP)
//   - Add trace_id / span_id correlation from context
//   - Add log sampling for high-throughput paths

package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Logger wraps slog.Logger and carries the service name as a permanent field.
// Using a wrapper (rather than the global slog) means each service instance
// has its own logger — critical for testing and for future per-service tuning.
type Logger struct {
	inner       *slog.Logger
	serviceName string
}

// New creates a structured JSON logger.
// level should be one of: "debug", "info", "warn", "error".
func New(serviceName, level string) *Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     l,
		AddSource: false, // set true in debug builds if needed
	})

	inner := slog.New(handler).With(
		slog.String("service", serviceName),
	)

	return &Logger{inner: inner, serviceName: serviceName}
}

// With returns a child logger with additional permanent key-value pairs.
// Use this to attach request_id, user_id, etc. to a request-scoped logger.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		inner:       l.inner.With(args...),
		serviceName: l.serviceName,
	}
}

// Info logs an informational message with optional key-value pairs.
func (l *Logger) Info(msg string, args ...any) {
	l.inner.Info(msg, args...)
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, args ...any) {
	l.inner.Debug(msg, args...)
}

// Warn logs a warning.
func (l *Logger) Warn(msg string, args ...any) {
	l.inner.Warn(msg, args...)
}

// Error logs an error.
func (l *Logger) Error(msg string, args ...any) {
	l.inner.Error(msg, args...)
}

// InfoContext logs with a context (future: extracts trace_id from ctx).
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.inner.InfoContext(ctx, msg, args...)
}

// ErrorContext logs an error with context.
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.inner.ErrorContext(ctx, msg, args...)
}
