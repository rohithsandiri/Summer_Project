// internal/operator/planner/planner_test.go

package planner

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

func TestPlannerGeneratePlan(t *testing.T) {
	m := metrics.New()
	p := NewPlanner(m)
	ctx := context.Background()

	policy := &models.Policy{
		ID:                "p1",
		Service:           "payment-service",
		AlertName:         "HighErrorRate",
		RecommendedAction: models.ActionPrepareRollback,
		Timeout:           8 * time.Minute,
		CooldownDuration:  6 * time.Minute,
		MaxRetries:        4,
	}

	decision := &models.DecisionEntry{
		Decision:   models.ActionPrepareRollback,
		PolicyUsed: "p1",
		Reason:     "Policy matched",
	}

	incident := &models.Incident{
		Service: "payment-service",
	}

	plan, err := p.Plan(ctx, incident, decision, policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan == nil {
		t.Fatal("Expected plan to be generated, got nil")
	}

	if plan.ServiceID != "payment-service" {
		t.Errorf("Expected service payment-service, got %s", plan.ServiceID)
	}

	if plan.DesiredAction != models.ActionPrepareRollback {
		t.Errorf("Expected action Prepare Rollback, got %s", plan.DesiredAction)
	}

	if plan.Priority != 1 {
		t.Errorf("Expected rollback priority 1, got %d", plan.Priority)
	}

	if plan.Timeout != 8*time.Minute {
		t.Errorf("Expected timeout 8m, got %v", plan.Timeout)
	}

	if plan.Cooldown != 6*time.Minute {
		t.Errorf("Expected cooldown 6m, got %v", plan.Cooldown)
	}

	if plan.ExecutionState != "Pending" {
		t.Errorf("Expected execution state Pending, got %s", plan.ExecutionState)
	}
}
