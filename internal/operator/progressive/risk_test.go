// internal/operator/progressive/risk_test.go

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
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

func TestRiskEngine(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	graph := dependency.NewGraph()
	promMock := prometheus.NewMockClient()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999, // 0.1% allowed error rate
		},
	}

	budgetMgr := budget.NewManager(slos, promMock, m)
	burnEngine := burnrate.NewEngine(slos, promMock, m)

	engine := NewRiskEngine(graph, budgetMgr, burnEngine, rollbackStore)

	// Case 1: Healthy system -> Low risk
	promMock.TrafficValues["payment-service"] = 10.0
	promMock.ErrorRateValues["payment-service"] = 0.0001
	promMock.AvailabilityValues["payment-service"] = 0.9995

	res, err := engine.CalculateRisk(ctx, "payment-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.RiskScore > 30.0 {
		t.Errorf("expected low risk score, got %.2f", res.RiskScore)
	}

	if res.Decision != "Deploy" {
		t.Errorf("expected Decision to be Deploy, got %s", res.Decision)
	}

	// Case 2: Elevated burn rate -> Medium/High risk
	promMock.ErrorRateValues["payment-service"] = 0.02 // burn rate = 20.0 (>14.4)
	promMock.AvailabilityValues["payment-service"] = 0.98

	res2, _ := engine.CalculateRisk(ctx, "payment-service")
	if res2.RiskScore < 30.0 {
		t.Errorf("expected higher risk score for elevated burn rate, got %.2f", res2.RiskScore)
	}
}
