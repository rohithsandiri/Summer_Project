// internal/operator/rootcause/analyzer.go
//
// Root Cause Analyzer. Resolves dependency graph error patterns to identify true faulty node.

package rootcause

import (
	"context"

	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

type RootCauseAnalysis struct {
	AlertedServiceID   string
	RootCauseServiceID string
	Reason             string
	ConfidenceScore    float64
}

type RootCauseAnalyzer interface {
	Analyze(ctx context.Context, alertedService string) (*RootCauseAnalysis, error)
}

type Analyzer struct {
	graph     dependency.DependencyGraph
	sloEngine slo.SLOEvaluator
	promAgent prometheus.PrometheusClient
	m         *metrics.OperatorMetrics
}

func NewAnalyzer(
	graph dependency.DependencyGraph,
	sloEngine slo.SLOEvaluator,
	promAgent prometheus.PrometheusClient,
	m *metrics.OperatorMetrics,
) *Analyzer {
	return &Analyzer{
		graph:     graph,
		sloEngine: sloEngine,
		promAgent: promAgent,
		m:         m,
	}
}

func (a *Analyzer) Analyze(ctx context.Context, alertedService string) (*RootCauseAnalysis, error) {
	a.m.DependencyChecksTotal.Inc()

	// 1. Fetch downstream services of the alerted service
	downstream := a.graph.GetDownstream(alertedService)
	if len(downstream) == 0 {
		// Alerted service has no downstream dependencies, it must be the root cause itself
		a.m.RootCauseTotal.WithLabelValues(alertedService, alertedService).Inc()
		return &RootCauseAnalysis{
			AlertedServiceID:   alertedService,
			RootCauseServiceID: alertedService,
			Reason:             "Service is a leaf node in the system topology.",
			ConfidenceScore:    1.0,
		}, nil
	}

	// 2. Evaluate SLO status of downstream services
	type unhealthyNode struct {
		service   string
		errorRate float64
		latency   float64
	}
	var degradedNodes []unhealthyNode

	for _, ds := range downstream {
		report, err := a.sloEngine.Evaluate(ctx, ds)
		if err != nil {
			continue // skip if we can't evaluate
		}

		if report.AnyViolation {
			degradedNodes = append(degradedNodes, unhealthyNode{
				service:   ds,
				errorRate: report.ErrorRateActual,
				latency:   report.LatencyActual,
			})
		}
	}

	// If no downstream dependencies are violated, the alerted service itself is the source of failure
	if len(degradedNodes) == 0 {
		a.m.RootCauseTotal.WithLabelValues(alertedService, alertedService).Inc()
		return &RootCauseAnalysis{
			AlertedServiceID:   alertedService,
			RootCauseServiceID: alertedService,
			Reason:             "All downstream dependencies are performing within SLO targets.",
			ConfidenceScore:    0.9,
		}, nil
	}

	// 3. Find the deepest/most degraded downstream dependency
	// In Gateway -> Order -> Payment hierarchy, if Gateway is alerted, and both Order and Payment are degraded,
	// Payment is the root cause because it has no downstream and is failing.
	// Let's analyze if any degraded node is a child of another degraded node.
	rootCauseCandidate := degradedNodes[0].service
	maxDepth := 0

	for _, node := range degradedNodes {
		// Depth can be estimated by number of upstream parents (more parents = deeper)
		parents := a.graph.GetUpstream(node.service)
		if len(parents) > maxDepth {
			maxDepth = len(parents)
			rootCauseCandidate = node.service
		}
	}

	reason := "Downstream dependency " + rootCauseCandidate + " is violating SLOs, causing cascading failures upstream to " + alertedService
	a.m.RootCauseTotal.WithLabelValues(alertedService, rootCauseCandidate).Inc()

	return &RootCauseAnalysis{
		AlertedServiceID:   alertedService,
		RootCauseServiceID: rootCauseCandidate,
		Reason:             reason,
		ConfidenceScore:    0.85,
	}, nil
}
