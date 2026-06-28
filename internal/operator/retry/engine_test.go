// internal/operator/retry/engine_test.go

package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
)

func TestRetryEngineClassification(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	re := NewRetryEngine(m, log)

	// Transient failures (should return true)
	transientErrors := []error{
		errors.New("dial tcp: i/o timeout"),
		errors.New("connection reset by peer"),
		errors.New("service temporarily unavailable"),
		errors.New("unexpected EOF"),
	}

	// Non-transient failures (should return false)
	nonTransientErrors := []error{
		errors.New("helm release: not found"),
		errors.New("namespace not found in cluster"),
		errors.New("invalid namespace target"),
		errors.New("revision not found in history"),
		errors.New("no healthy historical revision found"),
	}

	for _, err := range transientErrors {
		if !re.IsTransient(err) {
			t.Errorf("expected error %q to be transient", err.Error())
		}
	}

	for _, err := range nonTransientErrors {
		if re.IsTransient(err) {
			t.Errorf("expected error %q to be non-transient", err.Error())
		}
	}
}

func TestRetryEngineCalculateBackoff(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	re := NewRetryEngine(m, log)

	base := 1 * time.Second

	// Attempt 0: base
	b0 := re.CalculateBackoff(0, base)
	if b0 != 1*time.Second {
		t.Errorf("expected 1s, got %v", b0)
	}

	// Attempt 1: 2s (base * 2^1)
	b1 := re.CalculateBackoff(1, base)
	if b1 != 2*time.Second {
		t.Errorf("expected 2s, got %v", b1)
	}

	// Attempt 2: 4s (base * 2^2)
	b2 := re.CalculateBackoff(2, base)
	if b2 != 4*time.Second {
		t.Errorf("expected 4s, got %v", b2)
	}

	// Capped at 5m
	bLarge := re.CalculateBackoff(100, base)
	if bLarge != 5*time.Minute {
		t.Errorf("expected cap of 5m, got %v", bLarge)
	}
}
