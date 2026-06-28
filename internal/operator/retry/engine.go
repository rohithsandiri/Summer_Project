// internal/operator/retry/engine.go
//
// Retry Engine implementing exponential backoff and transient failure classification.

package retry

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
)

type RetryEngine struct {
	m   *metrics.OperatorMetrics
	log *logger.Logger
}

func NewRetryEngine(m *metrics.OperatorMetrics, log *logger.Logger) *RetryEngine {
	return &RetryEngine{
		m:   m,
		log: log,
	}
}

// IsTransient evaluates whether the error is classified as transient (and thus retryable).
func (re *RetryEngine) IsTransient(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Non-transient failure conditions that should NEVER be retried
	nonTransientRules := []string{
		"release: not found",
		"release not found",
		"namespace not found",
		"invalid namespace",
		"revision not found",
		"no healthy historical revision",
		"invalid release name",
		"unauthorized",
		"forbidden",
	}

	for _, rule := range nonTransientRules {
		if strings.Contains(errStr, rule) {
			return false
		}
	}

	return true
}

// CalculateBackoff calculates the exponential backoff duration based on current retry count.
func (re *RetryEngine) CalculateBackoff(attempt int, baseBackoff time.Duration) time.Duration {
	if attempt <= 0 {
		return baseBackoff
	}

	// Exponential backoff: base * 2^attempt
	factor := math.Pow(2, float64(attempt))
	backoff := time.Duration(float64(baseBackoff) * factor)

	// Cap the backoff at a maximum duration (e.g. 5 minutes)
	maxBackoff := 5 * time.Minute
	if backoff > maxBackoff {
		return maxBackoff
	}

	return backoff
}

// ExecuteWithRetry encapsulates executing a function with exponential backoff retries.
func (re *RetryEngine) ExecuteWithRetry(
	ctx context.Context,
	service string,
	maxRetries int,
	baseBackoff time.Duration,
	operation func() error,
	traceID string,
) error {
	var lastErr error
	f := logger.Fields{
		TraceID: traceID,
		Service: service,
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			re.m.RetryTotal.WithLabelValues(service, "recovery_retry").Inc()
			backoff := re.CalculateBackoff(attempt-1, baseBackoff)
			re.log.Info(ctx, "sleeping before retry attempt", f, "attempt", attempt, "backoff", backoff.String())

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err
		re.log.Warn(ctx, "operation failed", f, "attempt", attempt, "error", err.Error())

		if !re.IsTransient(err) {
			re.log.Error(ctx, "non-transient failure encountered, aborting retries", f, "error", err.Error())
			return err
		}
	}

	return lastErr
}
