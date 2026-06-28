// internal/operator/logger/logger.go
//
// Structured logger ensuring all operator logs contain:
// Trace ID, Incident ID, Alert Name, Service, Policy, Decision, State, and Reason.

package logger

import (
	"context"
	"fmt"
	"os"
	"time"
)

type Fields struct {
	TraceID    string
	IncidentID string
	AlertName  string
	Service    string
	Policy     string
	Decision   string
	State      string
	Reason     string
}

type Logger struct {
	version string
}

func New(version string) *Logger {
	return &Logger{version: version}
}

// Log logs a message with structured fields in a clean key-value format.
func (l *Logger) Log(ctx context.Context, level, msg string, f Fields, extra ...any) {
	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Format core fields
	logStr := fmt.Sprintf("[%s] LEVEL=%s MSG=%q TraceID=%s IncidentID=%s AlertName=%s Service=%s Policy=%s Decision=%s State=%s Reason=%q version=%s",
		timestamp,
		level,
		msg,
		l.emptyOrVal(f.TraceID),
		l.emptyOrVal(f.IncidentID),
		l.emptyOrVal(f.AlertName),
		l.emptyOrVal(f.Service),
		l.emptyOrVal(f.Policy),
		l.emptyOrVal(f.Decision),
		l.emptyOrVal(f.State),
		f.Reason,
		l.version,
	)

	// Append any extra key-values
	if len(extra) > 0 {
		for i := 0; i < len(extra); i += 2 {
			if i+1 < len(extra) {
				logStr += fmt.Sprintf(" %s=%v", extra[i], extra[i+1])
			} else {
				logStr += fmt.Sprintf(" %v", extra[i])
			}
		}
	}

	// Write to stderr or stdout
	if level == "ERROR" || level == "FATAL" {
		fmt.Fprintln(os.Stderr, logStr)
		if level == "FATAL" {
			os.Exit(1)
		}
	} else {
		fmt.Fprintln(os.Stdout, logStr)
	}
}

func (l *Logger) Info(ctx context.Context, msg string, f Fields, extra ...any) {
	l.Log(ctx, "INFO", msg, f, extra...)
}

func (l *Logger) Warn(ctx context.Context, msg string, f Fields, extra ...any) {
	l.Log(ctx, "WARN", msg, f, extra...)
}

func (l *Logger) Error(ctx context.Context, msg string, f Fields, extra ...any) {
	l.Log(ctx, "ERROR", msg, f, extra...)
}

func (l *Logger) Fatal(ctx context.Context, msg string, f Fields, extra ...any) {
	l.Log(ctx, "FATAL", msg, f, extra...)
}

func (l *Logger) emptyOrVal(val string) string {
	if val == "" {
		return "-"
	}
	return val
}
