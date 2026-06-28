// internal/operator/progressive/analysis_test.go

package progressive

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

func TestAnalysisEngine(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.300,
			ErrorRateMax:       0.01,
			ThroughputMin:      0.0,
		},
	}

	sloEngine := slo.NewEngine(slos, promMock, m)
	budgetMgr := budget.NewManager(slos, promMock, m)
	burnEngine := burnrate.NewEngine(slos, promMock, m)

	engine := NewAnalysisEngine(promMock, sloEngine, budgetMgr, burnEngine)

	// Case 1: healthy analysis
	promMock.AvailabilityValues["payment-service"] = 0.995
	promMock.LatencyValues["payment-service"] = 0.150
	promMock.ErrorRateValues["payment-service"] = 0.001
	promMock.TrafficValues["payment-service"] = 10.0

	res, err := engine.AnalyzeDeployment(ctx, "payment-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Healthy {
		t.Errorf("expected candidate to be healthy, got unhealthy: %s", res.Reason)
	}

	// Case 2: unhealthy analysis (SLO violation)
	promMock.LatencyValues["payment-service"] = 0.450
	res2, _ := engine.AnalyzeDeployment(ctx, "payment-service")
	if res2.Healthy {
		t.Errorf("expected candidate to be unhealthy due to latency violation")
	}
}
