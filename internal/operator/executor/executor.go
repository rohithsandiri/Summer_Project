// internal/operator/executor/executor.go
//
// Recovery Executor interface and HelmRollbackExecutor implementation.
// Decouples recovery logic to avoid giant switch statements.

package executor

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

// RecoveryExecutor defines the execution interface for any recovery action.
type RecoveryExecutor interface {
	Execute(ctx context.Context, plan *models.RecoveryPlan, incident *models.Incident, traceID string) (*models.RecoveryResult, error)
}

// HelmRollbackExecutor executes a Helm Rollback using the Helm Go SDK.
type HelmRollbackExecutor struct {
	helmClient   helm.HelmManager
	stateMachine interfaces.StateMachine
	m            *metrics.OperatorMetrics
	log          *logger.Logger
}

func NewHelmRollbackExecutor(
	helmClient helm.HelmManager,
	stateMachine interfaces.StateMachine,
	m *metrics.OperatorMetrics,
	log *logger.Logger,
) *HelmRollbackExecutor {
	return &HelmRollbackExecutor{
		helmClient:   helmClient,
		stateMachine: stateMachine,
		m:            m,
		log:          log,
	}
}

// Execute performs the rollback operation.
func (e *HelmRollbackExecutor) Execute(
	ctx context.Context,
	plan *models.RecoveryPlan,
	incident *models.Incident,
	traceID string,
) (*models.RecoveryResult, error) {
	startTime := time.Now().UTC()

	releaseName, namespace := ResolveReleaseAndNamespace(plan.ServiceID)

	f := logger.Fields{
		TraceID:    traceID,
		IncidentID: incident.ID,
		Service:    plan.ServiceID,
		Reason:     fmt.Sprintf("Executing Helm Rollback for release: %s in namespace: %s", releaseName, namespace),
	}

	e.log.Info(ctx, "initiating helm rollback execution", f)
	e.m.ActiveRecoveries.WithLabelValues(plan.ServiceID).Inc()
	defer e.m.ActiveRecoveries.WithLabelValues(plan.ServiceID).Dec()

	// 1. Transition state machine to ExecutingRollback
	if err := e.stateMachine.Transition(ctx, incident, models.StateExecutingRollback, "Initiating Helm Rollback"); err != nil {
		return nil, fmt.Errorf("failed to transition to ExecutingRollback: %w", err)
	}

	// 2. Fetch current version
	currRev, err := e.helmClient.GetCurrentRevision(namespace, releaseName)
	if err != nil {
		e.m.FailedRollbacksTotal.WithLabelValues(plan.ServiceID, namespace).Inc()
		return nil, fmt.Errorf("failed to get current helm revision: %w", err)
	}

	// 3. Fetch last known healthy revision (intelligent history check)
	targetRev, err := e.helmClient.GetLastHealthyRevision(namespace, releaseName)
	if err != nil {
		e.m.FailedRollbacksTotal.WithLabelValues(plan.ServiceID, namespace).Inc()
		return nil, fmt.Errorf("failed to determine rollback target revision: %w", err)
	}

	f.Reason = fmt.Sprintf("Rollback target determined. Current: v%d, Target: v%d", currRev, targetRev)
	e.log.Info(ctx, "determined healthy rollback target", f)

	// Update target revision in the plan for tracking
	plan.TargetRevision = fmt.Sprintf("v%d", targetRev)

	// Increment rollbacks metric
	e.m.RollbacksTotal.WithLabelValues(plan.ServiceID, namespace).Inc()

	// 4. Perform Rollback
	rollbackErr := e.helmClient.Rollback(namespace, releaseName, targetRev)
	duration := time.Since(startTime)

	if rollbackErr != nil {
		e.m.FailedRollbacksTotal.WithLabelValues(plan.ServiceID, namespace).Inc()
		return &models.RecoveryResult{
			Success:          false,
			OldRevision:      currRev,
			RollbackRevision: targetRev,
			Message:          rollbackErr.Error(),
			ExecutionTime:    duration,
		}, rollbackErr
	}

	// 5. Transition state machine to RollbackComplete
	if err := e.stateMachine.Transition(ctx, incident, models.StateRollbackComplete, fmt.Sprintf("Rollback to revision v%d finished successfully", targetRev)); err != nil {
		return nil, fmt.Errorf("failed to transition to RollbackComplete: %w", err)
	}

	return &models.RecoveryResult{
		Success:          true,
		OldRevision:      currRev,
		RollbackRevision: targetRev,
		Message:          fmt.Sprintf("Successfully rolled back %s from v%d to v%d", releaseName, currRev, targetRev),
		ExecutionTime:    duration,
	}, nil
}

// Helper to resolve Helm Release name and Namespace
func ResolveReleaseAndNamespace(service string) (string, string) {
	namespace := os.Getenv("TARGET_NAMESPACE")
	if namespace == "" {
		namespace = "cloud-native-platform"
	}
	return service, namespace
}

// ─── Future Executors (Placeholders) ────────────────────────────────────────

type RestartExecutor struct{}

func (e *RestartExecutor) Execute(ctx context.Context, plan *models.RecoveryPlan, incident *models.Incident, traceID string) (*models.RecoveryResult, error) {
	return &models.RecoveryResult{Success: true, Message: "Restart executed (no-op stub)"}, nil
}

type ScaleExecutor struct{}

func (e *ScaleExecutor) Execute(ctx context.Context, plan *models.RecoveryPlan, incident *models.Incident, traceID string) (*models.RecoveryResult, error) {
	return &models.RecoveryResult{Success: true, Message: "Scale executed (no-op stub)"}, nil
}

type CanaryExecutor struct{}

func (e *CanaryExecutor) Execute(ctx context.Context, plan *models.RecoveryPlan, incident *models.Incident, traceID string) (*models.RecoveryResult, error) {
	return &models.RecoveryResult{Success: true, Message: "Canary rollback executed (no-op stub)"}, nil
}
