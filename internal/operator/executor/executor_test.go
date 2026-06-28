// internal/operator/executor/executor_test.go

package executor

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/state"
	"helm.sh/helm/v3/pkg/release"
)

func TestHelmRollbackExecutorSuccess(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	stateMachine := state.NewStateMachine(log, m, nil)
	helmClient := helm.NewMockHelmClient()

	relKey := "cloud-native-platform/order-service"
	helmClient.CurrentVersion[relKey] = 2
	helmClient.Releases[relKey] = []*release.Release{
		{
			Name:      "order-service",
			Namespace: "cloud-native-platform",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		},
		{
			Name:      "order-service",
			Namespace: "cloud-native-platform",
			Version:   2,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		},
	}

	exec := NewHelmRollbackExecutor(helmClient, stateMachine, m, log)

	plan := &models.RecoveryPlan{
		ID:            "plan-123",
		ServiceID:     "order-service",
		DesiredAction: models.ActionPrepareRollback,
		Timeout:       2 * time.Second,
	}

	incident := &models.Incident{
		ID:           "inc-123",
		Service:      "order-service",
		AlertName:    "HighErrorRate",
		CurrentState: models.StateRecoveryPlanned,
	}

	res, err := exec.Execute(ctx, plan, incident, "trace-id-123")
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success to be true")
	}

	if res.OldRevision != 2 || res.RollbackRevision != 1 {
		t.Errorf("expected rollback from v2 to v1, got v%d to v%d", res.OldRevision, res.RollbackRevision)
	}

	if incident.CurrentState != models.StateRollbackComplete {
		t.Errorf("expected state to be StateRollbackComplete, got %s", incident.CurrentState)
	}
}

func TestHelmRollbackExecutorFailure(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	stateMachine := state.NewStateMachine(log, m, nil)
	helmClient := helm.NewMockHelmClient()
	helmClient.FailOnRollback = true // trigger failure

	relKey := "cloud-native-platform/order-service"
	helmClient.CurrentVersion[relKey] = 2
	helmClient.Releases[relKey] = []*release.Release{
		{
			Name:      "order-service",
			Namespace: "cloud-native-platform",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		},
		{
			Name:      "order-service",
			Namespace: "cloud-native-platform",
			Version:   2,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		},
	}

	exec := NewHelmRollbackExecutor(helmClient, stateMachine, m, log)

	plan := &models.RecoveryPlan{
		ID:            "plan-123",
		ServiceID:     "order-service",
		DesiredAction: models.ActionPrepareRollback,
		Timeout:       2 * time.Second,
	}

	incident := &models.Incident{
		ID:           "inc-123",
		Service:      "order-service",
		AlertName:    "HighErrorRate",
		CurrentState: models.StateRecoveryPlanned,
	}

	_, err := exec.Execute(ctx, plan, incident, "trace-id-123")
	if err == nil {
		t.Errorf("expected execution error, got nil")
	}
}
