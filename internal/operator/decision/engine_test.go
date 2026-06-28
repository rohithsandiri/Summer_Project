// internal/operator/decision/engine_test.go

package decision

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/rootcause"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

func TestIntelligentDecisionEngine(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	promMock := prometheus.NewMockClient()
	graph := dependency.NewGraph()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999, // 99.9%
			LatencyP95Max:      0.300, // 300ms
			ErrorRateMax:       0.01,  // 1%
			ThroughputMin:      10.0,
		},
	}

	sloEngine := slo.NewEngine(slos, promMock, m)
	budgetMgr := budget.NewManager(slos, promMock, m)
	burnRateEngine := burnrate.NewEngine(slos, promMock, m)
	rootcauseAnal := rootcause.NewAnalyzer(graph, sloEngine, promMock, m)

	engine := NewEngine(
		"1.0.0",
		m,
		sloEngine,
		budgetMgr,
		burnRateEngine,
		rootcauseAnal,
		rollbackStore,
	)

	policy := &models.Policy{
		ID:                "policy-payment",
		Service:           "payment-service",
		AlertName:         "HighErrorRate",
		RecommendedAction: models.ActionPrepareRollback,
		MaxRetries:        3,
		CooldownDuration:  5 * time.Minute,
	}

	alert := &models.Alert{
		Name:    "HighErrorRate",
		Service: "payment-service",
		Status:  "firing",
	}

	// Case 1: Low burn rate alert -> Decision should be ActionWait (observe)
	promMock.TrafficValues["payment-service"] = 100.0
	promMock.ErrorRateValues["payment-service"] = 0.0005 // very low error rate
	promMock.AvailabilityValues["payment-service"] = 0.9995

	entry, err := engine.MakeDecision(ctx, alert, policy, nil, false)
	if err != nil {
		t.Fatalf("unexpected error making decision: %v", err)
	}

	if entry.Decision != models.ActionWait {
		t.Errorf("expected low burn rate decision to be ActionWait, got %s", entry.Decision)
	}

	// Case 2: High burn rate -> Decision should be ActionPrepareRollback (rollback immediately)
	promMock.ErrorRateValues["payment-service"] = 0.05 // 5% errors (burn rate = 0.05 / 0.001 = 50.0 > 14.4)
	promMock.AvailabilityValues["payment-service"] = 0.95

	entry2, _ := engine.MakeDecision(ctx, alert, policy, nil, false)
	if entry2.Decision != models.ActionPrepareRollback {
		t.Errorf("expected high burn rate decision to be ActionPrepareRollback, got %s (reason: %q)", entry2.Decision, entry2.Reason)
	}

	// Case 3: Recovery history indicates consecutive failures -> Escalate
	_ = rollbackStore.RecordRollback(ctx, &models.RollbackHistory{
		Service:            "payment-service",
		VerificationResult: "Failed",
	})
	_ = rollbackStore.RecordRollback(ctx, &models.RollbackHistory{
		Service:            "payment-service",
		VerificationResult: "Failed",
	})

	entry3, _ := engine.MakeDecision(ctx, alert, policy, nil, false)
	if entry3.Decision != models.ActionEscalate {
		t.Errorf("expected consecutive rollback failures to trigger ActionEscalate, got %s (reason: %q)", entry3.Decision, entry3.Reason)
	}
}
