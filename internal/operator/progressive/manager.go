// internal/operator/progressive/manager.go

package progressive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/argo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/executor"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type ProgressiveDeliveryManager interface {
	CreateRollout(ctx context.Context, name, namespace, service string) (*models.DeploymentHistory, error)
	MonitorRollout(ctx context.Context, name, namespace string) error
	PauseRollout(ctx context.Context, name, namespace string) error
	ResumeRollout(ctx context.Context, name, namespace string) error
	PromoteRollout(ctx context.Context, name, namespace string) error
	AbortRollout(ctx context.Context, name, namespace string) error
	ListDeployments(ctx context.Context) ([]*models.DeploymentHistory, error)
	GetDeployment(ctx context.Context, id string) (*models.DeploymentHistory, error)
}

type DeliveryManager struct {
	argoClient RolloutManager
	guard      DeploymentGuard
	riskEngine DeploymentRiskEngine
	analyzer   DeploymentAnalysisEngine
	verifier   ReleaseVerificationEngine

	// Existing self-healing recovery engine for rollback execution
	recoveryExecutor executor.RecoveryExecutor

	log *logger.Logger
	m   *metrics.OperatorMetrics

	mu         sync.RWMutex
	history    map[string]*models.DeploymentHistory
	activeRuns map[string]*models.DeploymentHistory
}

type RolloutManager interface {
	GetRollout(ctx context.Context, name, namespace string) (*argo.RolloutDetails, error)
	PauseRollout(ctx context.Context, name, namespace string) error
	ResumeRollout(ctx context.Context, name, namespace string) error
	AbortRollout(ctx context.Context, name, namespace string) error
	PromoteRollout(ctx context.Context, name, namespace string) error
}

func NewDeliveryManager(
	argoClient RolloutManager,
	guard DeploymentGuard,
	riskEngine DeploymentRiskEngine,
	analyzer DeploymentAnalysisEngine,
	verifier ReleaseVerificationEngine,
	recoveryExecutor executor.RecoveryExecutor,
	log *logger.Logger,
	m *metrics.OperatorMetrics,
) *DeliveryManager {
	return &DeliveryManager{
		argoClient:       argoClient,
		guard:            guard,
		riskEngine:       riskEngine,
		analyzer:         analyzer,
		verifier:         verifier,
		recoveryExecutor: recoveryExecutor,
		log:              log,
		m:                m,
		history:          make(map[string]*models.DeploymentHistory),
		activeRuns:       make(map[string]*models.DeploymentHistory),
	}
}

func (dm *DeliveryManager) CreateRollout(ctx context.Context, name, namespace, service string) (*models.DeploymentHistory, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.m.RolloutsTotal.Inc()

	// 1. Evaluate Deployment Guard checks
	allowed, reason, err := dm.guard.EvaluateGuard(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate guard: %w", err)
	}

	if !allowed {
		dm.m.DeploymentGuardBlocksTotal.Inc()
		return nil, fmt.Errorf("deployment blocked by guard policies: %s", reason)
	}

	// 2. Calculate initial risk score
	riskDetails, err := dm.riskEngine.CalculateRisk(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate risk: %w", err)
	}

	dm.m.DeploymentRiskScore.Set(riskDetails.RiskScore)

	if riskDetails.Decision == "Reject" {
		dm.m.DeploymentGuardBlocksTotal.Inc()
		return nil, fmt.Errorf("deployment rejected due to high risk score: %.2f (%v)", riskDetails.RiskScore, riskDetails.Reasons)
	}

	// 3. Register history record
	historyID := fmt.Sprintf("deploy-%s-%d", name, time.Now().Unix())
	historyRecord := &models.DeploymentHistory{
		DeploymentID:     historyID,
		Revision:         1,
		HelmRelease:      service,
		ArgoRollout:      name,
		Strategy:         "Canary",
		StartTime:        time.Now().UTC(),
		PromotionResult:  "Pending",
		RiskScore:        riskDetails.RiskScore,
		OperatorDecision: riskDetails.Decision,
		CurrentState:     models.RolloutStateStarting,
	}

	dm.history[historyID] = historyRecord
	dm.activeRuns[fmt.Sprintf("%s/%s", namespace, name)] = historyRecord

	dm.log.Info(ctx, "started progressive delivery rollout", logger.Fields{
		Service: service,
		Reason:  "initial guard checks passed",
	}, "deployment_id", historyID, "strategy", "Canary", "risk_score", riskDetails.RiskScore)

	return historyRecord, nil
}

func (dm *DeliveryManager) MonitorRollout(ctx context.Context, name, namespace string) error {
	key := fmt.Sprintf("%s/%s", namespace, name)

	dm.mu.Lock()
	record, exists := dm.activeRuns[key]
	dm.mu.Unlock()

	if !exists {
		return fmt.Errorf("no active progressive rollout monitoring running for %s", key)
	}

	// Fetch actual rollout details from Argo client
	ro, err := dm.argoClient.GetRollout(ctx, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to get argo rollout details: %w", err)
	}

	// Transition state machine
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if ro.Aborted {
		record.CurrentState = models.RolloutStateAborted
		record.PromotionResult = "Aborted"
		record.EndTime = time.Now().UTC()
		delete(dm.activeRuns, key)
		return nil
	}

	if ro.CurrentWeight >= 100 {
		record.CurrentState = models.RolloutStateCompleted
		record.PromotionResult = "Promoted"
		record.EndTime = time.Now().UTC()
		delete(dm.activeRuns, key)
		dm.m.PromotionsTotal.Inc()
		return nil
	}

	// Continuous canary analysis evaluation loop
	analysis, err := dm.analyzer.AnalyzeDeployment(ctx, record.HelmRelease)
	if err != nil {
		return err
	}

	dm.m.CanaryAnalysisTotal.Inc()

	if !analysis.Healthy {
		dm.m.CanaryFailuresTotal.Inc()
		// Auto-abort rollout
		dm.log.Warn(ctx, "canary analysis degraded; triggering automatic abort", logger.Fields{
			Service: record.HelmRelease,
			Reason:  analysis.Reason,
		})

		record.CurrentState = models.RolloutStateAborted
		record.EndTime = time.Now().UTC()
		record.PromotionResult = "Aborted"

		// Set abort on Argo rollout
		_ = dm.argoClient.AbortRollout(ctx, name, namespace)

		// Invoke existing Helm Rollback recovery engine
		dm.m.AbortsTotal.Inc()
		delete(dm.activeRuns, key)

		// Execute Helm Rollback asynchronously via RecoveryExecutor
		go func() {
			plan := &models.RecoveryPlan{
				ID:            fmt.Sprintf("abort-%s", record.HelmRelease),
				ServiceID:     record.HelmRelease,
				DesiredAction: models.ActionPrepareRollback,
				Timeout:       5 * time.Minute,
			}
			incident := &models.Incident{
				ID:           fmt.Sprintf("abort-%s", record.HelmRelease),
				Service:      record.HelmRelease,
				CurrentState: models.StateWarning,
			}
			_, _ = dm.recoveryExecutor.Execute(context.Background(), plan, incident, "progressive-abort")
		}()

		return nil
	}

	dm.m.CanarySuccessTotal.Inc()

	// If healthy, proceed to promote rollout step if verified
	verified, reason, err := dm.verifier.VerifyRelease(ctx, record.HelmRelease, namespace)
	if err == nil && verified {
		record.CurrentState = models.RolloutStatePromoting
		_ = dm.argoClient.PromoteRollout(ctx, name, namespace)
	} else {
		// Keep paused to analyze
		record.CurrentState = models.RolloutStatePaused
		_ = dm.argoClient.PauseRollout(ctx, name, namespace)
		dm.log.Info(ctx, "rollout verification pending or paused", logger.Fields{
			Service: record.HelmRelease,
			Reason:  reason,
		})
	}

	return nil
}

func (dm *DeliveryManager) PauseRollout(ctx context.Context, name, namespace string) error {
	return dm.argoClient.PauseRollout(ctx, name, namespace)
}

func (dm *DeliveryManager) ResumeRollout(ctx context.Context, name, namespace string) error {
	return dm.argoClient.ResumeRollout(ctx, name, namespace)
}

func (dm *DeliveryManager) PromoteRollout(ctx context.Context, name, namespace string) error {
	return dm.argoClient.PromoteRollout(ctx, name, namespace)
}

func (dm *DeliveryManager) AbortRollout(ctx context.Context, name, namespace string) error {
	return dm.argoClient.AbortRollout(ctx, name, namespace)
}

func (dm *DeliveryManager) ListDeployments(ctx context.Context) ([]*models.DeploymentHistory, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	res := make([]*models.DeploymentHistory, 0, len(dm.history))
	for _, h := range dm.history {
		res = append(res, h)
	}
	return res, nil
}

func (dm *DeliveryManager) GetDeployment(ctx context.Context, id string) (*models.DeploymentHistory, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	h, ok := dm.history[id]
	if !ok {
		return nil, fmt.Errorf("deployment %s not found", id)
	}
	return h, nil
}
