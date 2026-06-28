// internal/operator/progressive/guard_test.go

package progressive

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

func TestDeploymentGuard(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	graph := dependency.NewGraph()
	incidentStore := storage.NewInMemoryIncidentStore()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.5,
			ErrorRateMax:       0.05,
		},
	}

	sloEngine := slo.NewEngine(slos, promMock, m)
	budgetMgr := budget.NewManager(slos, promMock, m)
	burnEngine := burnrate.NewEngine(slos, promMock, m)

	cfg := models.DeploymentGuardConfig{
		MaxAllowedBurnRate:       14.4,
		MinRemainingBudget:       10.0,
		BlockOnCriticalIncidents: true,
	}

	guard := NewGuard(cfg, incidentStore, graph, sloEngine, budgetMgr, burnEngine)

	// Case 1: normal healthy conditions -> allowed
	promMock.TrafficValues["payment-service"] = 100.0
	promMock.ErrorRateValues["payment-service"] = 0.001
	promMock.AvailabilityValues["payment-service"] = 0.999
	promMock.LatencyValues["payment-service"] = 0.1

	allowed, reason, err := guard.EvaluateGuard(ctx, "payment-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !allowed {
		t.Errorf("expected guard evaluation to allow, got blocked: %s", reason)
	}

	// Case 2: active critical incident -> blocked
	_ = incidentStore.Create(ctx, &models.Incident{
		ID:           "inc-1",
		Service:      "payment-service",
		Status:       "firing",
		CurrentState: models.StateWarning,
	})

	allowed2, reason2, _ := guard.EvaluateGuard(ctx, "payment-service")
	if allowed2 {
		t.Errorf("expected guard evaluation to block, got allowed: %s", reason2)
	}
}
