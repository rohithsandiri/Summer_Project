// internal/operator/budget/manager_test.go

package budget

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

func TestErrorBudgetCalculation(t *testing.T) {
	ctx := context.Background()
	m := metrics.New()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99, // 99% availability (allows 1% error budget)
		},
	}

	mgr := NewManager(slos, promMock, m)

	// Set metrics
	promMock.TrafficValues["payment-service"] = 10.0    // 10 requests per second
	promMock.ErrorRateValues["payment-service"] = 0.005 // 0.5% errors (consuming half of the allowed budget)

	// Window is 24 hours
	report, err := mgr.EvaluateBudget(ctx, "payment-service", 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTotal := 10.0 * 24 * 3600
	if report.TotalRequests != expectedTotal {
		t.Errorf("expected total requests %v, got %v", expectedTotal, report.TotalRequests)
	}

	// Allowed is 1% of total
	expectedAllowed := expectedTotal * 0.01
	if math.Abs(report.AllowedErrors-expectedAllowed) > 0.001 {
		t.Errorf("expected allowed errors %v, got %v", expectedAllowed, report.AllowedErrors)
	}

	// Consumed is 0.5% of total
	expectedConsumed := expectedTotal * 0.005
	if math.Abs(report.ConsumedErrors-expectedConsumed) > 0.001 {
		t.Errorf("expected consumed errors %v, got %v", expectedConsumed, report.ConsumedErrors)
	}

	// Remaining percentage should be 50%
	if math.Abs(report.RemainingPercent-50.0) > 0.001 {
		t.Errorf("expected remaining budget percentage 50.0, got %v", report.RemainingPercent)
	}
}
