// internal/operator/state/machine_test.go

package state

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

func TestStateMachineTransitions(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	sm := NewStateMachine(log, m, nil)
	ctx := context.Background()

	incident := &models.Incident{
		ID:           "inc-test",
		CurrentState: models.StateHealthy,
		Service:      "payment-service",
		AlertName:    "HighErrorRate",
	}

	// 1. Valid: Healthy -> Warning
	err := sm.Transition(ctx, incident, models.StateWarning, "alert started firing")
	if err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}
	if incident.CurrentState != models.StateWarning {
		t.Errorf("Expected StateWarning, got %s", incident.CurrentState)
	}

	// 2. Valid: Warning -> Investigating
	err = sm.Transition(ctx, incident, models.StateInvestigating, "evaluating policies")
	if err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// 3. Invalid: Investigating -> Healthy (must go through Recovered)
	err = sm.Transition(ctx, incident, models.StateHealthy, "jump state")
	if err == nil {
		t.Fatal("Expected validation error for invalid transition")
	}
	if incident.CurrentState != models.StateInvestigating {
		t.Errorf("State modified on failed transition: got %s", incident.CurrentState)
	}
}
