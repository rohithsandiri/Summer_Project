// internal/operator/decision/engine.go
//
// Intelligent SRE Decision Engine. Incorporates SLO status, error budget,
// burn rates, cascading dependency roots, and historical rollback success rate
// to make optimal recovery choices.

package decision

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/rootcause"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
)

type Engine struct {
	version        string
	m              *metrics.OperatorMetrics
	sloEngine      slo.SLOEvaluator
	budgetMgr      budget.ErrorBudgetManager
	burnRateEngine burnrate.BurnRateEngine
	rootcauseAnal  rootcause.RootCauseAnalyzer
	rollbackStore  interfaces.RollbackHistoryStore
}

func NewEngine(
	version string,
	m *metrics.OperatorMetrics,
	sloEngine slo.SLOEvaluator,
	budgetMgr budget.ErrorBudgetManager,
	burnRateEngine burnrate.BurnRateEngine,
	rootcauseAnal rootcause.RootCauseAnalyzer,
	rollbackStore interfaces.RollbackHistoryStore,
) *Engine {
	return &Engine{
		version:        version,
		m:              m,
		sloEngine:      sloEngine,
		budgetMgr:      budgetMgr,
		burnRateEngine: burnRateEngine,
		rootcauseAnal:  rootcauseAnal,
		rollbackStore:  rollbackStore,
	}
}

// MakeDecision determines the healing Action based on SRE metrics, history, and dependency root cause.
func (e *Engine) MakeDecision(
	ctx context.Context,
	alert *models.Alert,
	policy *models.Policy,
	incident *models.Incident,
	isCoolingDown bool,
) (*models.DecisionEntry, error) {

	entry := &models.DecisionEntry{
		Timestamp:       time.Now().UTC(),
		OperatorVersion: e.version,
	}

	// 1. Alert is resolved
	if alert.Status == "resolved" {
		entry.Decision = models.ActionIgnore
		entry.Reason = "Alert has been resolved"
		e.m.DecisionsTotal.WithLabelValues(alert.Service, string(entry.Decision), "none").Inc()
		e.m.DecisionConfidence.Set(1.0)
		return entry, nil
	}

	// 2. No matching policy found
	if policy == nil {
		entry.Decision = models.ActionIgnore
		entry.Reason = fmt.Sprintf("No active policy found for alert: %s on service: %s", alert.Name, alert.Service)
		e.m.DecisionsTotal.WithLabelValues(alert.Service, string(entry.Decision), "none").Inc()
		e.m.DecisionConfidence.Set(0.5)
		return entry, nil
	}

	entry.PolicyUsed = policy.ID

	// 3. System is currently in cooldown window
	if isCoolingDown {
		entry.Decision = models.ActionWait
		entry.Reason = "Service is in a cooldown window to prevent thrashing"
		e.m.DecisionsTotal.WithLabelValues(alert.Service, string(entry.Decision), policy.ID).Inc()
		e.m.DecisionConfidence.Set(0.9)
		return entry, nil
	}

	// 4. Max retries exceeded — Escalate
	if incident != nil && incident.RecoveryAttempts >= policy.MaxRetries {
		entry.Decision = models.ActionEscalate
		entry.Reason = fmt.Sprintf("Maximum recovery attempts (%d) reached. Escalating to human operators.", policy.MaxRetries)
		e.m.DecisionsTotal.WithLabelValues(alert.Service, string(entry.Decision), policy.ID).Inc()
		e.m.DecisionConfidence.Set(1.0)
		return entry, nil
	}

	// 5. Intelligent Root Cause Analysis
	targetService := alert.Service
	rcAnalysis, rcErr := e.rootcauseAnal.Analyze(ctx, alert.Service)
	if rcErr == nil && rcAnalysis != nil {
		targetService = rcAnalysis.RootCauseServiceID
		entry.RecoveryTargetServiceID = targetService
	}

	// 6. Evaluate SRE signals for the target service
	sloReport, sloErr := e.sloEngine.Evaluate(ctx, targetService)
	budgetReport, budgetErr := e.budgetMgr.EvaluateBudget(ctx, targetService, 24*time.Hour)

	var burnRateReport *burnrate.BurnRateReport
	var burnRateErr error
	if budgetErr == nil && budgetReport != nil {
		burnRateReport, burnRateErr = e.burnRateEngine.CalculateBurnRate(ctx, targetService, 1*time.Hour, budgetReport.RemainingPercent)
	}

	// 7. Recovery History Analysis
	rollbacks, histErr := e.rollbackStore.ListRollbacks(ctx)
	consecutiveFailures := 0
	totalRollbacks := 0
	successfulRollbacks := 0

	if histErr == nil {
		for _, r := range rollbacks {
			if r.Service == targetService {
				totalRollbacks++
				if r.VerificationResult == "Success" {
					successfulRollbacks++
					consecutiveFailures = 0
				} else {
					consecutiveFailures++
				}
			}
		}
	}

	// 8. Make SRE-driven decision rules
	action := policy.RecommendedAction
	reason := fmt.Sprintf("Policy matched: %s. Recommending action: %s", policy.ID, policy.RecommendedAction)
	confidence := 0.8 // default confidence

	if rcAnalysis != nil && rcAnalysis.RootCauseServiceID != alert.Service {
		reason = fmt.Sprintf("Cascading failure detected. Service %q is impacted, but true root cause is resolved as %q. %s", alert.Service, rcAnalysis.RootCauseServiceID, rcAnalysis.Reason)
		confidence = rcAnalysis.ConfidenceScore
	}

	if consecutiveFailures >= 2 {
		action = models.ActionEscalate
		reason = fmt.Sprintf("Consecutive rollbacks (%d) failed for service %s. Escalating for manual SRE intervention.", consecutiveFailures, targetService)
		confidence = 1.0
	} else if burnRateErr == nil && burnRateReport != nil && burnRateReport.CurrentBurnRate < 2.0 && sloErr == nil && sloReport != nil && !sloReport.AvailabilityViolated {
		// Low burn rate and availability target not violated -> Wait/observe instead of disruptive rollback
		action = models.ActionWait
		reason = fmt.Sprintf("Target service %s availability within limits, and burn rate is low (%.2f). Observing for transient spike recovery.", targetService, burnRateReport.CurrentBurnRate)
		confidence = 0.85
	} else if burnRateErr == nil && burnRateReport != nil && burnRateReport.IsHighAlert {
		action = models.ActionPrepareRollback
		reason = fmt.Sprintf("Critical burn rate threshold (%.2f) exceeded for service %s. Initiating immediate automated rollback.", burnRateReport.CurrentBurnRate, targetService)
		confidence = 0.95
	} else if budgetErr == nil && budgetReport != nil && budgetReport.RemainingPercent <= 0 {
		action = models.ActionPrepareRollback
		reason = fmt.Sprintf("Error budget completely exhausted (0%% remaining) for service %s. Initiating emergency rollback.", targetService)
		confidence = 0.98
	}

	entry.Decision = action
	entry.Reason = reason

	e.m.DecisionsTotal.WithLabelValues(alert.Service, string(entry.Decision), policy.ID).Inc()
	e.m.DecisionConfidence.Set(confidence)

	return entry, nil
}
