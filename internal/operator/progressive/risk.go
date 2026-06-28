// internal/operator/progressive/risk.go

package progressive

import (
	"context"
	"math"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type DeploymentRiskEngine interface {
	CalculateRisk(ctx context.Context, service string) (*models.RiskAnalysisDetails, error)
}

type RiskEngine struct {
	graph         dependency.DependencyGraph
	budgetMgr     budget.ErrorBudgetManager
	burnEngine    burnrate.BurnRateEngine
	rollbackStore interfaces.RollbackHistoryStore
}

func NewRiskEngine(
	graph dependency.DependencyGraph,
	budgetMgr budget.ErrorBudgetManager,
	burnEngine burnrate.BurnRateEngine,
	rollbackStore interfaces.RollbackHistoryStore,
) *RiskEngine {
	return &RiskEngine{
		graph:         graph,
		budgetMgr:     budgetMgr,
		burnEngine:    burnEngine,
		rollbackStore: rollbackStore,
	}
}

func (re *RiskEngine) CalculateRisk(ctx context.Context, service string) (*models.RiskAnalysisDetails, error) {
	var reasons []string
	score := 10.0 // Base risk score

	// 1. Check Error Budget Remaining
	budgetReport, err := re.budgetMgr.EvaluateBudget(ctx, service, 24*time.Hour)
	if err == nil {
		rem := budgetReport.RemainingPercent
		if rem < 20.0 {
			score += 30.0
			reasons = append(reasons, "Remaining error budget is critically low")
		} else if rem < 50.0 {
			score += 15.0
			reasons = append(reasons, "Remaining error budget is under 50%")
		}
	}

	// 2. Check Burn Rate
	burnReport, err := re.burnEngine.CalculateBurnRate(ctx, service, 1*time.Hour, budgetReport.RemainingPercent)
	if err == nil {
		br := burnReport.CurrentBurnRate
		if br > 14.4 {
			score += 40.0
			reasons = append(reasons, "Error budget burn rate is critically high (>14.4)")
		} else if br > 6.0 {
			score += 20.0
			reasons = append(reasons, "Error budget burn rate is elevated (>6.0)")
		}
	}

	// 3. Check Rollback History
	rollbacks, err := re.rollbackStore.ListRollbacks(ctx)
	if err == nil {
		failCount := 0
		for _, r := range rollbacks {
			if r.Service == service && r.VerificationResult == "Failed" {
				failCount++
			}
		}
		if failCount > 0 {
			multiplier := math.Min(float64(failCount)*10.0, 30.0)
			score += multiplier
			reasons = append(reasons, "Service has a history of failed recoveries")
		}
	}

	// 4. Check Dependency Health
	downstream := re.graph.GetDownstream(service)
	if len(downstream) > 0 {
		score += float64(len(downstream)) * 5.0
	}

	// Cap score to [0, 100]
	if score > 100.0 {
		score = 100.0
	}
	if score < 0.0 {
		score = 0.0
	}

	// Determine decision mapping based on risk score thresholds
	decision := "Deploy"
	if score >= 85.0 {
		decision = "Reject"
	} else if score >= 60.0 {
		decision = "Pause"
	} else if score >= 30.0 {
		decision = "Canary"
	}

	return &models.RiskAnalysisDetails{
		RiskScore: score,
		Decision:  decision,
		Reasons:   reasons,
	}, nil
}
