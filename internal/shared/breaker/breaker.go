// internal/shared/breaker/breaker.go
//
// WHY THIS EXISTS:
//   In a highly distributed microservice architecture, slow downstreams or
//   outages can quickly exhaust threads/connections in upstream services.
//   A circuit breaker stops calling a failing downstream immediately once a
//   failure threshold is reached, failing fast and allowing the downstream to
//   recover.
//
// DESIGN:
//   We implement a classic state machine with three states:
//     - Closed: Traffic flows normally. If error rate > threshold, open circuit.
//     - Open: Traffic fails fast immediately without making the downstream call.
//             After recoveryInterval, transition to Half-Open.
//     - Half-Open: Allow a single trial request. If it succeeds, close circuit.
//                  If it fails, reopen circuit.

package breaker

import (
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// State represents the circuit breaker state.
type State string

const (
	StateClosed   State = "CLOSED"
	StateOpen     State = "OPEN"
	StateHalfOpen State = "HALF_OPEN"
)

var (
	// BreakerState reports the current state of named breakers (0=Closed, 1=Half-Open, 2=Open).
	BreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Current state of circuit breakers: 0=CLOSED, 1=HALF_OPEN, 2=OPEN",
		},
		[]string{"name"},
	)

	// BreakerRejections counts requests blocked by the breaker without calling the downstream.
	BreakerRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "circuit_breaker_rejections_total",
			Help: "Total requests blocked and failed-fast by the circuit breaker.",
		},
		[]string{"name"},
	)
)

// ErrCircuitOpen is returned when calls are rejected because the breaker is OPEN.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config controls circuit breaker thresholds and intervals.
type Config struct {
	Name             string
	FailureThreshold int           // number of consecutive failures before opening
	RecoveryInterval time.Duration // time to wait in OPEN state before trying again
}

// CircuitBreaker manages execution wrapping with state transitions.
type CircuitBreaker struct {
	mu  sync.RWMutex
	cfg Config

	state            State
	consecutiveFails int
	lastStateChange  time.Time
}

// New creates a named CircuitBreaker.
func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = 30 * time.Second
	}

	cb := &CircuitBreaker{
		cfg:             cfg,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}

	cb.updateMetrics()
	return cb
}

// Execute wraps an operation. If the circuit is open, it returns ErrCircuitOpen.
// If the call succeeds, metrics are updated. If it fails, failure counters increment.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.allowRequest() {
		BreakerRejections.WithLabelValues(cb.cfg.Name).Inc()
		return ErrCircuitOpen
	}

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.handleFailure()
		return err
	}

	cb.handleSuccess()
	return nil
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	state := cb.state
	lastChange := cb.lastStateChange
	cb.mu.RUnlock()

	if state == StateClosed {
		return true
	}

	if state == StateOpen {
		// If recovery interval has passed, transition to half-open to allow trial
		if time.Since(lastChange) > cb.cfg.RecoveryInterval {
			cb.mu.Lock()
			if cb.state == StateOpen {
				cb.state = StateHalfOpen
				cb.lastStateChange = time.Now()
				cb.updateMetrics()
			}
			cb.mu.Unlock()
			return true
		}
		return false
	}

	// In Half-Open state, allow the request to pass through
	return true
}

// Must be called with lock held.
func (cb *CircuitBreaker) handleSuccess() {
	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.consecutiveFails = 0
		cb.lastStateChange = time.Now()
		cb.updateMetrics()
	} else if cb.state == StateClosed {
		cb.consecutiveFails = 0
	}
}

// Must be called with lock held.
func (cb *CircuitBreaker) handleFailure() {
	cb.consecutiveFails++
	if cb.state == StateClosed && cb.consecutiveFails >= cb.cfg.FailureThreshold {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		cb.updateMetrics()
	} else if cb.state == StateHalfOpen {
		// Single trial failed — go straight back to open
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		cb.updateMetrics()
	}
}

// Must be called with lock held or from constructor.
func (cb *CircuitBreaker) updateMetrics() {
	var val float64
	switch cb.state {
	case StateClosed:
		val = 0
	case StateHalfOpen:
		val = 1
	case StateOpen:
		val = 2
	}
	BreakerState.WithLabelValues(cb.cfg.Name).Set(val)
}

// GetState returns the current state (thread-safe).
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
