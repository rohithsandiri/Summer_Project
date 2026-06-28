// internal/operator/slo/engine.go
//
// SLO Evaluation Engine. Matches Prometheus golden signals against configured targets.

package slo

import (
	"context"
	"fmt"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
)

type SLOReport struct {
	ServiceID            string
	AvailabilitySLO      float64
	AvailabilityActual   float64
	AvailabilityViolated bool
	LatencySLO           float64
	LatencyActual        float64
	LatencyViolated      bool
	ErrorRateSLO         float64
	ErrorRateActual      float64
	ErrorRateViolated    bool
	ThroughputSLO        float64
	ThroughputActual     float64
	ThroughputViolated   bool
	AnyViolation         bool
}

type SLOEvaluator interface {
	Evaluate(ctx context.Context, service string) (*SLOReport, error)
	GetSLO(service string) (*models.SLO, bool)
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

func (e *Engine) GetSLO(service string) (*models.SLO, bool) {
	s, ok := e.slos[service]
	return s, ok
}

func (e *Engine) Evaluate(ctx context.Context, service string) (*SLOReport, error) {
	sloObj, ok := e.slos[service]
	if !ok {
		return nil, fmt.Errorf("no SLO configuration registered for service %q", service)
	}

	// Fetch current metrics
	availability, err := e.promAgent.GetAvailability(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to query availability for %q: %w", service, err)
	}

	latency, err := e.promAgent.GetLatency(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to query latency for %q: %w", service, err)
	}

	errorRate, err := e.promAgent.GetErrorRate(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to query error rate for %q: %w", service, err)
	}

	throughput, err := e.promAgent.GetTraffic(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to query throughput for %q: %w", service, err)
	}

	// Compare actuals against targets
	availabilityViolated := availability < sloObj.AvailabilityTarget
	latencyViolated := latency > sloObj.LatencyP95Max
	errorRateViolated := errorRate > sloObj.ErrorRateMax

	// Throughput is only checked for violation if traffic exists
	throughputViolated := throughput < sloObj.ThroughputMin && throughput > 0

	anyViolation := availabilityViolated || latencyViolated || errorRateViolated || throughputViolated

	if anyViolation {
		e.m.SLOViolationsTotal.WithLabelValues(service).Inc()
	}

	return &SLOReport{
		ServiceID:            service,
		AvailabilitySLO:      sloObj.AvailabilityTarget,
		AvailabilityActual:   availability,
		AvailabilityViolated: availabilityViolated,
		LatencySLO:           sloObj.LatencyP95Max,
		LatencyActual:        latency,
		LatencyViolated:      latencyViolated,
		ErrorRateSLO:         sloObj.ErrorRateMax,
		ErrorRateActual:      errorRate,
		ErrorRateViolated:    errorRateViolated,
		ThroughputSLO:        sloObj.ThroughputMin,
		ThroughputActual:     throughput,
		ThroughputViolated:   throughputViolated,
		AnyViolation:         anyViolation,
	}, nil
}
