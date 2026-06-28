// internal/shared/breaker/breaker_test.go

package breaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := New(Config{
		Name:             "test-breaker",
		FailureThreshold: 2,
		RecoveryInterval: 50 * time.Millisecond,
	})

	if cb.GetState() != StateClosed {
		t.Errorf("expected initially CLOSED state, got %s", cb.GetState())
	}

	// 1st failure
	_ = cb.Execute(func() error {
		return errors.New("transient error")
	})
	if cb.GetState() != StateClosed {
		t.Errorf("expected CLOSED state after 1 failure, got %s", cb.GetState())
	}

	// 2nd failure — should open the circuit
	_ = cb.Execute(func() error {
		return errors.New("transient error")
	})
	if cb.GetState() != StateOpen {
		t.Errorf("expected OPEN state after 2 failures, got %s", cb.GetState())
	}

	// Immediate execution should fail fast with ErrCircuitOpen
	err := cb.Execute(func() error {
		return nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for recovery interval to pass
	time.Sleep(60 * time.Millisecond)

	// Next execution should transition to HALF_OPEN and pass through
	passed := false
	err = cb.Execute(func() error {
		passed = true
		return nil
	})
	if err != nil {
		t.Errorf("expected successful execution, got error %v", err)
	}
	if !passed {
		t.Error("expected mock operation to execute")
	}

	// Successful execution should close the circuit
	if cb.GetState() != StateClosed {
		t.Errorf("expected CLOSED state after successful test, got %s", cb.GetState())
	}
}
