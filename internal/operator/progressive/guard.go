// internal/operator/progressive/guard.go

package progressive

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

type DeploymentGuard interface {
	EvaluateGuard(ctx context.Context, service string) (bool, string, error)
}

type Guard struct {
	config        models.DeploymentGuardConfig
	incidentStore interfaces.IncidentStore
	graph         dependency.DependencyGraph
	sloEngine     slo.SLOEvaluator
	budgetMgr     budget.ErrorBudgetManager
	burnEngine    burnrate.BurnRateEngine
}

func NewGuard(
	cfg models.DeploymentGuardConfig,
	incidentStore interfaces.IncidentStore,
	graph dependency.DependencyGraph,
	sloEngine slo.SLOEvaluator,
	budgetMgr budget.ErrorBudgetManager,
	burnEngine burnrate.BurnRateEngine,
) *Guard {
	return &Guard{
		config:        cfg,
		incidentStore: incidentStore,
		graph:         graph,
		sloEngine:     sloEngine,
		budgetMgr:     budgetMgr,
		burnEngine:    burnEngine,
	}
}

func (g *Guard) EvaluateGuard(ctx context.Context, service string) (bool, string, error) {
	// 1. Check Active Critical Incidents
	if g.config.BlockOnCriticalIncidents {
		activeIncidents, err := g.incidentStore.ListActive(ctx)
		if err == nil {
			for _, inc := range activeIncidents {
				if inc.Service == service && inc.CurrentState != models.StateHealthy {
					return false, fmt.Sprintf("Blocked: active critical incident (%s) is ongoing for this service", inc.ID), nil
				}
			}
		}
	}

	// 2. Check Error Budget Remaining
	budgetReport, err := g.budgetMgr.EvaluateBudget(ctx, service, 24*time.Hour)
	if err == nil && budgetReport.RemainingPercent < g.config.MinRemainingBudget {
		return false, fmt.Sprintf("Blocked: remaining error budget (%.2f%%) is below configured minimum threshold (%.2f%%)", budgetReport.RemainingPercent, g.config.MinRemainingBudget), nil
	}

	// 3. Check Burn Rate
	burnReport, err := g.burnEngine.CalculateBurnRate(ctx, service, 1*time.Hour, budgetReport.RemainingPercent)
	if err == nil && burnReport.CurrentBurnRate > g.config.MaxAllowedBurnRate {
		return false, fmt.Sprintf("Blocked: error budget burn rate (%.2f) exceeds configured maximum threshold (%.2f)", burnReport.CurrentBurnRate, g.config.MaxAllowedBurnRate), nil
	}

	// 4. Check Dependency Health
	downstream := g.graph.GetDownstream(service)
	for _, ds := range downstream {
		sloRep, err := g.sloEngine.Evaluate(ctx, ds)
		if err == nil && sloRep.AnyViolation {
			return false, fmt.Sprintf("Blocked: downstream dependency %s is violating SLO targets", ds), nil
		}
	}

	return true, "Allowed: deployment matches all guard checks", nil
}
