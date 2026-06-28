// internal/operator/budget/manager.go
//
// Error Budget Engine. Computes consumed and remaining error budgets over various SRE windows.

package budget

import (
	"context"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

type BudgetReport struct {
	ServiceID          string
	Window             time.Duration
	AvailabilityTarget float64
	TotalRequests      float64
	AllowedErrors      float64
	ConsumedErrors     float64
	RemainingAbsolute  float64
	RemainingPercent   float64
}

type ErrorBudgetManager interface {
	EvaluateBudget(ctx context.Context, service string, window time.Duration) (*BudgetReport, error)
}

type Manager struct {
	slos      map[string]*models.SLO
	promAgent prometheus.PrometheusClient
	m         *metrics.OperatorMetrics
}

func NewManager(slos []models.SLO, promAgent prometheus.PrometheusClient, m *metrics.OperatorMetrics) *Manager {
	sloMap := make(map[string]*models.SLO)
	for _, s := range slos {
		clone := s
		sloMap[s.ServiceID] = &clone
	}

	return &Manager{
		slos:      sloMap,
		promAgent: promAgent,
		m:         m,
	}
}

func (mgr *Manager) EvaluateBudget(ctx context.Context, service string, window time.Duration) (*BudgetReport, error) {
	sloObj, ok := mgr.slos[service]
	if !ok {
		// Default SLO targets if not specified to prevent nil crashes
		sloObj = &models.SLO{
			ServiceID:          service,
			AvailabilityTarget: 0.99,
		}
	}

	// Fetch current error rate and throughput (RPS)
	errRate, err := mgr.promAgent.GetErrorRate(ctx, service)
	if err != nil {
		return nil, err
	}

	traffic, err := mgr.promAgent.GetTraffic(ctx, service)
	if err != nil {
		return nil, err
	}

	// Total requests expected in the window duration assuming steady-state current traffic
	windowSeconds := window.Seconds()
	totalRequests := traffic * windowSeconds

	// Target failure budget
	unavailabilityTarget := 1.0 - sloObj.AvailabilityTarget
	allowedErrors := totalRequests * unavailabilityTarget
	consumedErrors := totalRequests * errRate

	remainingAbsolute := allowedErrors - consumedErrors
	if remainingAbsolute < 0 {
		remainingAbsolute = 0
	}

	remainingPercent := 100.0
	if allowedErrors > 0 {
		remainingPercent = (remainingAbsolute / allowedErrors) * 100.0
	}

	// Expose remaining budget metrics
	windowStr := window.String()
	mgr.m.ErrorBudgetRemaining.WithLabelValues(service, windowStr).Set(remainingPercent)

	return &BudgetReport{
		ServiceID:          service,
		Window:             window,
		AvailabilityTarget: sloObj.AvailabilityTarget,
		TotalRequests:      totalRequests,
		AllowedErrors:      allowedErrors,
		ConsumedErrors:     consumedErrors,
		RemainingAbsolute:  remainingAbsolute,
		RemainingPercent:   remainingPercent,
	}, nil
}
