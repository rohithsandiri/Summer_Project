// internal/operator/burnrate/engine.go
//
// Burn Rate Engine. Calculates error budget consumption speed and time to exhaustion.

package burnrate

import (
	"context"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

type BurnRateReport struct {
	ServiceID           string
	Window              time.Duration
	CurrentBurnRate     float64
	ProjectedExhaustion time.Duration
	IsHighAlert         bool
}

type BurnRateEngine interface {
	CalculateBurnRate(ctx context.Context, service string, window time.Duration, remainingBudgetPercent float64) (*BurnRateReport, error)
}

type Engine struct {
	slos      map[string]*models.SLO
	promAgent prometheus.PrometheusClient
	m         *metrics.OperatorMetrics
}

func NewEngine(slos []models.SLO, promAgent prometheus.PrometheusClient, m *metrics.OperatorMetrics) *Engine {
	sloMap := make(map[string]*models.SLO)
	for _, s := range slos {
		clone := s
		sloMap[s.ServiceID] = &clone
	}

	return &Engine{
		slos:      sloMap,
		promAgent: promAgent,
		m:         m,
	}
}

func (e *Engine) CalculateBurnRate(ctx context.Context, service string, window time.Duration, remainingBudgetPercent float64) (*BurnRateReport, error) {
	sloObj, ok := e.slos[service]
	if !ok {
		sloObj = &models.SLO{
			ServiceID:          service,
			AvailabilityTarget: 0.999, // default 99.9%
		}
	}

	// Fetch actual error rate
	errRate, err := e.promAgent.GetErrorRate(ctx, service)
	if err != nil {
		return nil, err
	}

	allowedFailureRate := 1.0 - sloObj.AvailabilityTarget
	if allowedFailureRate <= 0 {
		allowedFailureRate = 0.001 // prevent divide-by-zero
	}

	// Burn Rate = Actual Error Rate / Allowed Failure Rate
	burnRate := errRate / allowedFailureRate

	// Time to exhaustion based on 30-day (720h) budget window
	const totalBudgetWindowHours = 30.0 * 24.0
	var projectedExhaustion time.Duration

	if burnRate <= 0 {
		// No errors being consumed
		projectedExhaustion = 1000 * 24 * time.Hour // essentially infinite
	} else {
		// Exhaustion Hours = (Remaining Budget % / 100) * Total Window Hours / Burn Rate
		hours := (remainingBudgetPercent / 100.0) * totalBudgetWindowHours / burnRate
		if hours > 1000.0 {
			hours = 1000.0
		}
		projectedExhaustion = time.Duration(hours * float64(time.Hour))
	}

	// Google SRE multi-window burn rate alert thresholds:
	// - 1h window: Burn Rate > 14.4 (consuming 2% in 1 hour)
	// - 6h window: Burn Rate > 6.0 (consuming 5% in 6 hours)
	isHighAlert := false
	if window <= 1*time.Hour && burnRate > 14.4 {
		isHighAlert = true
	} else if window <= 6*time.Hour && burnRate > 6.0 {
		isHighAlert = true
	}

	// Update operator burn rate metrics
	windowStr := window.String()
	e.m.BurnRate.WithLabelValues(service, windowStr).Set(burnRate)

	return &BurnRateReport{
		ServiceID:           service,
		Window:              window,
		CurrentBurnRate:     burnRate,
		ProjectedExhaustion: projectedExhaustion,
		IsHighAlert:         isHighAlert,
	}, nil
}
