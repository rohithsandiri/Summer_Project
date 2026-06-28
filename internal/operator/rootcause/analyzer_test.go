// internal/operator/rootcause/analyzer_test.go

package rootcause

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

func TestRootCauseAnalysis(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	graph := dependency.NewGraph()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "api-gateway",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.200,
			ErrorRateMax:       0.02,
			ThroughputMin:      0.0,
		},
		{
			ServiceID:          "order-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.200,
			ErrorRateMax:       0.02,
			ThroughputMin:      0.0,
		},
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.300,
			ErrorRateMax:       0.02,
			ThroughputMin:      0.0,
		},
		{
			ServiceID:          "inventory-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.200,
			ErrorRateMax:       0.02,
			ThroughputMin:      0.0,
		},
	}

	sloEngine := slo.NewEngine(slos, promMock, m)
	analyzer := NewAnalyzer(graph, sloEngine, promMock, m)

	// Set clean default metrics for all mocked services to be perfectly within SLO targets
	for _, s := range []string{"api-gateway", "order-service", "payment-service", "inventory-service"} {
		promMock.AvailabilityValues[s] = 0.999
		promMock.LatencyValues[s] = 0.05
		promMock.ErrorRateValues[s] = 0.001
		promMock.TrafficValues[s] = 10.0
	}

	// Case 1: Healthy downstream -> Blame alerted service itself (api-gateway)
	promMock.AvailabilityValues["api-gateway"] = 0.95 // degraded

	res, err := analyzer.Analyze(ctx, "api-gateway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.RootCauseServiceID != "api-gateway" {
		t.Errorf("expected root cause to be api-gateway, got %s (reason: %q)", res.RootCauseServiceID, res.Reason)
	}

	// Case 2: Cascading Failure -> Gateway is slow/failing because Payment is degraded
	// Reset gateway availability to healthy
	promMock.AvailabilityValues["api-gateway"] = 0.999

	// Degrade both order-service and payment-service downstream
	promMock.AvailabilityValues["order-service"] = 0.95
	promMock.AvailabilityValues["payment-service"] = 0.90

	res2, err := analyzer.Analyze(ctx, "api-gateway")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should correctly identify the deepest culprit: payment-service
	if res2.RootCauseServiceID != "payment-service" {
		t.Errorf("expected root cause payment-service, got %s (reason: %q)", res2.RootCauseServiceID, res2.Reason)
	}
}
