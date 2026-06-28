// internal/operator/slo/engine_test.go

package slo

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

func TestSLOEngineEvaluation(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999, // 99.9%
			LatencyP95Max:      0.300, // 300ms
			ErrorRateMax:       0.01,  // 1%
			ThroughputMin:      10.0,
		},
	}

	engine := NewEngine(slos, promMock, m)

	// Case 1: Healthy (all targets met)
	promMock.AvailabilityValues["payment-service"] = 0.9995
	promMock.LatencyValues["payment-service"] = 0.150
	promMock.ErrorRateValues["payment-service"] = 0.001
	promMock.TrafficValues["payment-service"] = 15.0

	rep, err := engine.Evaluate(ctx, "payment-service")
	if err != nil {
		t.Fatalf("unexpected evaluation error: %v", err)
	}

	if rep.AnyViolation {
		t.Errorf("expected no violations, got: %+v", rep)
	}

	// Case 2: Unhealthy Latency
	promMock.LatencyValues["payment-service"] = 0.450
	rep, _ = engine.Evaluate(ctx, "payment-service")
	if !rep.LatencyViolated || !rep.AnyViolation {
		t.Errorf("expected Latency violation, got: %+v", rep)
	}

	// Reset latency, trigger Availability violation
	promMock.LatencyValues["payment-service"] = 0.150
	promMock.AvailabilityValues["payment-service"] = 0.990
	rep, _ = engine.Evaluate(ctx, "payment-service")
	if !rep.AvailabilityViolated || !rep.AnyViolation {
		t.Errorf("expected Availability violation, got: %+v", rep)
	}
}
