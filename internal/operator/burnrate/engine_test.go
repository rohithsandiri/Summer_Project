// internal/operator/burnrate/engine_test.go

package burnrate

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

func TestBurnRateCalculations(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999, // 99.9% target -> 0.1% allowed error rate
		},
	}

	engine := NewEngine(slos, promMock, m)

	// Case 1: 2% error rate -> Burn Rate = 2% / 0.1% = 20.0
	promMock.ErrorRateValues["payment-service"] = 0.02
	report, err := engine.CalculateBurnRate(ctx, "payment-service", 1*time.Hour, 100.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if math.Abs(report.CurrentBurnRate-20.0) > 0.001 {
		t.Errorf("expected burn rate 20.0, got %v", report.CurrentBurnRate)
	}

	// Projected Exhaustion time: 30 days / 20 = 1.5 days = 36 hours
	expectedExhaustion := 36 * time.Hour
	if report.ProjectedExhaustion != expectedExhaustion {
		t.Errorf("expected exhaustion time %v, got %v", expectedExhaustion, report.ProjectedExhaustion)
	}

	if !report.IsHighAlert {
		t.Errorf("expected high burn rate alert to trigger (20 > 14.4)")
	}

	// Case 2: Very healthy (0% error rate)
	promMock.ErrorRateValues["payment-service"] = 0.0
	report2, _ := engine.CalculateBurnRate(ctx, "payment-service", 1*time.Hour, 100.0)
	if report2.CurrentBurnRate != 0.0 {
		t.Errorf("expected burn rate 0, got %v", report2.CurrentBurnRate)
	}
	if report2.ProjectedExhaustion <= 100*time.Hour {
		t.Errorf("expected infinite exhaustion time for 0 burn rate, got %v", report2.ProjectedExhaustion)
	}
}
