// internal/operator/policy/engine_test.go

package policy

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

func TestPolicyEngineMatch(t *testing.T) {
	m := metrics.New()
	policies := []models.Policy{
		{
			ID:                "p1",
			Service:           "payment-service",
			AlertName:         "HighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
		},
	}

	engine := NewEngine(policies, m)
	ctx := context.Background()

	// Matching alert
	alertMatch := &models.Alert{
		Name:    "HighErrorRate",
		Service: "payment-service",
	}
	policy, matched := engine.Match(ctx, alertMatch)
	if !matched {
		t.Fatal("Expected policy match")
	}
	if policy.ID != "p1" {
		t.Errorf("Expected policy ID p1, got %s", policy.ID)
	}

	// Unmatching alert
	alertMismatch := &models.Alert{
		Name:    "HighLatency",
		Service: "payment-service",
	}
	_, matchedMismatch := engine.Match(ctx, alertMismatch)
	if matchedMismatch {
		t.Error("Expected no policy match")
	}
}
