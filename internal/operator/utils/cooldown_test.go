// internal/operator/utils/cooldown_test.go

package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTryStartExecutionSingleWinner verifies that when many goroutines race to
// start a recovery execution for the same service (per-pod alert instances
// arriving in one webhook batch), exactly one wins the slot.
func TestTryStartExecutionSingleWinner(t *testing.T) {
	cm := NewCooldownManager()
	ctx := context.Background()

	const racers = 50
	var wins int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if ok, _ := cm.TryStartExecution(ctx, "order-service", 5*time.Minute); ok {
				atomic.AddInt64(&wins, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", wins)
	}
}

func TestTryStartExecutionIndependentServices(t *testing.T) {
	cm := NewCooldownManager()
	ctx := context.Background()

	if ok, _ := cm.TryStartExecution(ctx, "order-service", time.Minute); !ok {
		t.Fatal("first execution for order-service should be allowed")
	}
	if ok, _ := cm.TryStartExecution(ctx, "payment-service", time.Minute); !ok {
		t.Fatal("execution for a different service must not be blocked")
	}
	if ok, remaining := cm.TryStartExecution(ctx, "order-service", time.Minute); ok || remaining <= 0 {
		t.Fatalf("second execution within window should be blocked with remaining > 0, got ok=%v remaining=%v", ok, remaining)
	}
}

func TestTryStartExecutionWindowExpiry(t *testing.T) {
	cm := NewCooldownManager()
	ctx := context.Background()

	if ok, _ := cm.TryStartExecution(ctx, "inventory-service", 10*time.Millisecond); !ok {
		t.Fatal("first execution should be allowed")
	}
	time.Sleep(20 * time.Millisecond)
	if ok, _ := cm.TryStartExecution(ctx, "inventory-service", time.Minute); !ok {
		t.Fatal("execution after window expiry should be allowed")
	}
}
