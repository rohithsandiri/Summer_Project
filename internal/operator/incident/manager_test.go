// internal/operator/incident/manager_test.go

package incident

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/decision"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/executor"
	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/planner"
	"github.com/rohithsandiri/Summer_Project/internal/operator/policy"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/reliability"
	"github.com/rohithsandiri/Summer_Project/internal/operator/retry"
	"github.com/rohithsandiri/Summer_Project/internal/operator/rootcause"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/state"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
	"github.com/rohithsandiri/Summer_Project/internal/operator/utils"
	"helm.sh/helm/v3/pkg/release"
)

type MockVerifier struct {
	VerifyFunc func(ctx context.Context, service, namespace, traceID string) (bool, error)
}

func (m *MockVerifier) Verify(ctx context.Context, service, namespace, traceID string) (bool, error) {
	if m.VerifyFunc != nil {
		return m.VerifyFunc(ctx, service, namespace, traceID)
	}
	return true, nil
}

func TestIncidentManagerOrchestration(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	// Configure mock/default policies
	policies := []models.Policy{
		{
			ID:                "p-test",
			Service:           "payment-service",
			AlertName:         "HighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			MaxRetries:        2,
			Timeout:           1 * time.Second, // short timeout for fast tests
		},
	}

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999,
			LatencyP95Max:      0.300,
			ErrorRateMax:       0.01,
			ThroughputMin:      10.0,
		},
	}

	store := storage.NewInMemoryIncidentStore()
	history := storage.NewInMemoryDecisionHistoryStore()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()
	timeline := reliability.NewInMemoryTimelineEngine()
	reliabilityEngine := reliability.NewReliabilityEngine(store, rollbackStore, timeline, nil)
	policyEngine := policy.NewEngine(policies, m)
	cooldown := utils.NewCooldownManager()

	promMock := prometheus.NewMockClient()
	// Mock high burn rate triggers to make sure rollback executes
	promMock.ErrorRateValues["payment-service"] = 0.05
	promMock.AvailabilityValues["payment-service"] = 0.95
	promMock.TrafficValues["payment-service"] = 100.0

	depGraph := dependency.NewGraph()
	sloEngine := slo.NewEngine(slos, promMock, m)
	budgetMgr := budget.NewManager(slos, promMock, m)
	burnRateEngine := burnrate.NewEngine(slos, promMock, m)
	rootcauseAnal := rootcause.NewAnalyzer(depGraph, sloEngine, promMock, m)

	decisionEngine := decision.NewEngine(
		"1.0.0",
		m,
		sloEngine,
		budgetMgr,
		burnRateEngine,
		rootcauseAnal,
		rollbackStore,
	)

	stateMachine := state.NewStateMachine(log, m, timeline)
	recoveryPlanner := planner.NewPlanner(m)

	// Set up mock Helm client
	helmClient := helm.NewMockHelmClient()
	relKey := "cloud-native-platform/payment-service"
	helmClient.CurrentVersion[relKey] = 2
	helmClient.Releases[relKey] = []*release.Release{
		{
			Name:      "payment-service",
			Namespace: "cloud-native-platform",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		},
		{
			Name:      "payment-service",
			Namespace: "cloud-native-platform",
			Version:   2,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		},
	}

	mockExecutor := executor.NewHelmRollbackExecutor(helmClient, stateMachine, m, log)
	mockVerifier := &MockVerifier{
		VerifyFunc: func(ctx context.Context, service, namespace, traceID string) (bool, error) {
			return true, nil
		},
	}
	retryEngine := retry.NewRetryEngine(m, log)

	mgr := NewManager(
		store,
		history,
		policyEngine,
		decisionEngine,
		recoveryPlanner,
		stateMachine,
		cooldown,
		log,
		m,
		mockExecutor,
		mockVerifier,
		retryEngine,
		rollbackStore,
		timeline,
		reliabilityEngine,
	)

	alertFiring := &models.Alert{
		Fingerprint: "fingerprint-123",
		Name:        "HighErrorRate",
		Service:     "payment-service",
		Severity:    "critical",
		Status:      "firing",
		StartsAt:    time.Now().Add(-1 * time.Minute),
	}

	// 1. Process new Firing alert (executes pipeline asynchronously)
	err := mgr.ProcessAlert(ctx, alertFiring, "trace-111")
	if err != nil {
		t.Fatalf("unexpected error processing alert: %v", err)
	}

	// Give background pipeline a moment to complete execution and verifications
	time.Sleep(500 * time.Millisecond)

	// Retrieve updated incident
	inc, err := store.GetByFingerprint(ctx, "fingerprint-123")
	if err != nil {
		t.Fatalf("failed to retrieve incident: %v", err)
	}

	if inc.Service != "payment-service" {
		t.Errorf("Expected payment-service, got %s", inc.Service)
	}

	// Active recoveries completed: state should transition to Healthy eventually
	if inc.CurrentState != models.StateHealthy {
		t.Errorf("Expected StateHealthy after successful recovery, got %s", inc.CurrentState)
	}

	rollbacks, err := rollbackStore.ListRollbacks(ctx)
	if err != nil || len(rollbacks) == 0 {
		t.Fatalf("expected recorded rollback history record, got %d records", len(rollbacks))
	}

	rollbackRecord := rollbacks[0]
	if rollbackRecord.OldRevision != 2 || rollbackRecord.RollbackRevision != 1 {
		t.Errorf("expected rollback from v2 to v1, got v%d to v%d", rollbackRecord.OldRevision, rollbackRecord.RollbackRevision)
	}
	if rollbackRecord.VerificationResult != "Success" {
		t.Errorf("expected verification success, got %q", rollbackRecord.VerificationResult)
	}

	// 2. Process Resolved alert
	alertResolved := &models.Alert{
		Fingerprint: "fingerprint-123",
		Name:        "HighErrorRate",
		Service:     "payment-service",
		Severity:    "critical",
		Status:      "resolved",
	}

	err = mgr.ProcessAlert(ctx, alertResolved, "trace-111")
	if err != nil {
		t.Fatalf("unexpected error processing resolved alert: %v", err)
	}

	incResolved, err := store.GetByFingerprint(ctx, "fingerprint-123")
	if err != nil {
		t.Fatalf("failed to retrieve incident: %v", err)
	}

	if incResolved.Status != "resolved" {
		t.Errorf("Expected status resolved, got %s", incResolved.Status)
	}
}
