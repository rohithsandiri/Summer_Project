// internal/operator/progressive/manager_test.go

package progressive

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/argo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/budget"
	"github.com/rohithsandiri/Summer_Project/internal/operator/burnrate"
	"github.com/rohithsandiri/Summer_Project/internal/operator/dependency"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

type mockRecoveryExecutor struct{}

func (m *mockRecoveryExecutor) Execute(ctx context.Context, plan *models.RecoveryPlan, incident *models.Incident, traceID string) (*models.RecoveryResult, error) {
	return &models.RecoveryResult{
		Success:       true,
		Message:       "Success",
		ExecutionTime: 1 * time.Second,
	}, nil
}

func TestDeliveryManager(t *testing.T) {
	ctx := context.Background()
	log := logger.New("test")
	m := metrics.New()
	graph := dependency.NewGraph()
	incidentStore := storage.NewInMemoryIncidentStore()
	promMock := prometheus.NewMockClient()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.5,
			ErrorRateMax:       0.05,
		},
	}

	sloEngine := slo.NewEngine(slos, promMock, m)
	budgetMgr := budget.NewManager(slos, promMock, m)
	burnEngine := burnrate.NewEngine(slos, promMock, m)

	cfg := models.DeploymentGuardConfig{
		MaxAllowedBurnRate:       14.4,
		MinRemainingBudget:       10.0,
		BlockOnCriticalIncidents: true,
	}

	guard := NewGuard(cfg, incidentStore, graph, sloEngine, budgetMgr, burnEngine)
	riskEngine := NewRiskEngine(graph, budgetMgr, burnEngine, rollbackStore)
	analysisEngine := NewAnalysisEngine(promMock, sloEngine, budgetMgr, burnEngine)

	// Mock argo client
	argoMock := argo.NewMockArgoClient()
	// Mock verification
	releaseVerifier := &mockReleaseVerificationEngine{verified: true}
	// Mock recovery executor
	recoveryExec := &mockRecoveryExecutor{}

	dm := NewDeliveryManager(
		argoMock,
		guard,
		riskEngine,
		analysisEngine,
		releaseVerifier,
		recoveryExec,
		log,
		m,
	)

	// Case 1: Start rollout successfully under low risk
	promMock.TrafficValues["payment-service"] = 100.0
	promMock.ErrorRateValues["payment-service"] = 0.001
	promMock.AvailabilityValues["payment-service"] = 0.999

	record, err := dm.CreateRollout(ctx, "payment-service-rollout", "default", "payment-service")
	if err != nil {
		t.Fatalf("unexpected error starting rollout: %v", err)
	}

	if record.PromotionResult != "Pending" {
		t.Errorf("expected PromotionResult Pending, got %s", record.PromotionResult)
	}

	// Case 2: Monitor and progress rollout
	err = dm.MonitorRollout(ctx, "payment-service-rollout", "default")
	if err != nil {
		t.Fatalf("unexpected error monitoring rollout: %v", err)
	}
}

type mockReleaseVerificationEngine struct {
	verified bool
}

func (m *mockReleaseVerificationEngine) VerifyRelease(ctx context.Context, service, namespace string) (bool, string, error) {
	return m.verified, "verified", nil
}
