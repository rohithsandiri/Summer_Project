// internal/operator/chaos/scenarios_test.go

package chaos

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/decision"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/executor"
	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/incident"
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

type MockVerifier struct{}

func (mv *MockVerifier) Verify(ctx context.Context, service string, namespace string, traceID string) (bool, error) {
	return true, nil
}

func TestAutomatedChaosScenarios(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	policies := []models.Policy{
		{
			ID:                "p-test-1",
			Service:           "payment-service",
			AlertName:         "HighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Second,
			MaxRetries:        2,
			Timeout:           1 * time.Second,
		},
		{
			ID:                "p-test-2",
			Service:           "inventory-service",
			AlertName:         "InventoryServiceDown",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Second,
			MaxRetries:        2,
			Timeout:           1 * time.Second,
		},
		{
			ID:                "p-test-3",
			Service:           "order-service",
			AlertName:         "DatabaseUnavailable",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Second,
			MaxRetries:        2,
			Timeout:           1 * time.Second,
		},
		{
			ID:                "p-test-4",
			Service:           "payment-service",
			AlertName:         "LatencyP95MaxViolation",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Second,
			MaxRetries:        2,
			Timeout:           1 * time.Second,
		},
	}

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.5,
			ErrorRateMax:       0.02,
		},
		{
			ServiceID:          "inventory-service",
			AvailabilityTarget: 0.99,
		},
		{
			ServiceID:          "order-service",
			AvailabilityTarget: 0.99,
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

	helmClient := helm.NewMockHelmClient()
	// Set mock versions for all services
	for _, svc := range []string{"payment-service", "inventory-service", "order-service"} {
		key := "cloud-native-platform/" + svc
		helmClient.CurrentVersion[key] = 2
		helmClient.Releases[key] = []*release.Release{
			{
				Name:      svc,
				Namespace: "cloud-native-platform",
				Version:   1,
				Info: &release.Info{
					Status: release.StatusDeployed,
				},
			},
			{
				Name:      svc,
				Namespace: "cloud-native-platform",
				Version:   2,
				Info: &release.Info{
					Status: release.StatusFailed,
				},
			},
		}
	}

	executorInstance := executor.NewHelmRollbackExecutor(helmClient, stateMachine, m, log)
	verifierInstance := &MockVerifier{}
	retryEngine := retry.NewRetryEngine(m, log)

	mgr := incident.NewManager(
		store,
		history,
		policyEngine,
		decisionEngine,
		recoveryPlanner,
		stateMachine,
		cooldown,
		log,
		m,
		executorInstance,
		verifierInstance,
		retryEngine,
		rollbackStore,
		timeline,
		reliabilityEngine,
	)

	scenarioEngine := NewScenarioEngine(store, rollbackStore, promMock, mgr, reliabilityEngine, timeline, log)

	// Run Scenario 1
	path1, err := scenarioEngine.RunScenario1(ctx)
	if err != nil {
		t.Fatalf("scenario 1 failed: %v", err)
	}
	if _, err := os.Stat(path1); err != nil {
		t.Errorf("scenario 1 report not generated: %v", err)
	}

	// Run Scenario 2
	path2, err := scenarioEngine.RunScenario2(ctx)
	if err != nil {
		t.Fatalf("scenario 2 failed: %v", err)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Errorf("scenario 2 report not generated: %v", err)
	}

	// Run Scenario 3
	path3, err := scenarioEngine.RunScenario3(ctx)
	if err != nil {
		t.Fatalf("scenario 3 failed: %v", err)
	}
	if _, err := os.Stat(path3); err != nil {
		t.Errorf("scenario 3 report not generated: %v", err)
	}

	// Run Scenario 4
	path4, err := scenarioEngine.RunScenario4(ctx)
	if err != nil {
		t.Fatalf("scenario 4 failed: %v", err)
	}
	if _, err := os.Stat(path4); err != nil {
		t.Errorf("scenario 4 report not generated: %v", err)
	}
}
