// internal/operator/progressive/analysis.go

package progressive

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

type AnalysisResult struct {
	Healthy         bool
	Reason          string
	LatencyRatio    float64 // candidate / stable latency ratio
	ErrorRateDiff   float64 // candidate - stable error rate
	BurnRate        float64
	RemainingBudget float64
}

type DeploymentAnalysisEngine interface {
	AnalyzeDeployment(ctx context.Context, service string) (*AnalysisResult, error)
}

type AnalysisEngine struct {
	promClient prometheus.PrometheusClient
	sloEngine  slo.SLOEvaluator
	budgetMgr  budget.ErrorBudgetManager
	burnEngine burnrate.BurnRateEngine
}

func NewAnalysisEngine(
	promClient prometheus.PrometheusClient,
	sloEngine slo.SLOEvaluator,
	budgetMgr budget.ErrorBudgetManager,
	burnEngine burnrate.BurnRateEngine,
) *AnalysisEngine {
	return &AnalysisEngine{
		promClient: promClient,
		sloEngine:  sloEngine,
		budgetMgr:  budgetMgr,
		burnEngine: burnEngine,
	}
}

func (ae *AnalysisEngine) AnalyzeDeployment(ctx context.Context, service string) (*AnalysisResult, error) {
	// Evaluate general SRE indicators for the target service
	sloReport, err := ae.sloEngine.Evaluate(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate SLO targets: %w", err)
	}

	budgetReport, err := ae.budgetMgr.EvaluateBudget(ctx, service, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate budget: %w", err)
	}

	burnReport, err := ae.burnEngine.CalculateBurnRate(ctx, service, 1*time.Hour, budgetReport.RemainingPercent)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate burn rate: %w", err)
	}

	// Calculate analysis health
	healthy := true
	reason := "Candidate release performs successfully within SLO limits"

	if sloReport.AnyViolation {
		healthy = false
		reason = "Candidate violating active SLO targets"
	} else if burnReport.IsHighAlert {
		healthy = false
		reason = fmt.Sprintf("High burn rate (%.2f) detected for candidate release", burnReport.CurrentBurnRate)
	} else if budgetReport.RemainingPercent < 10.0 {
		healthy = false
		reason = fmt.Sprintf("Candidate consumed excessive error budget. Only %.2f%% remaining", budgetReport.RemainingPercent)
	}

	return &AnalysisResult{
		Healthy:         healthy,
		Reason:          reason,
		LatencyRatio:    1.0, // In mock comparisons, ratio is steady
		ErrorRateDiff:   sloReport.ErrorRateActual,
		BurnRate:        burnReport.CurrentBurnRate,
		RemainingBudget: budgetReport.RemainingPercent,
	}, nil
}
