// internal/operator/webhook/handler_test.go

package webhook

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/alert"
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
)

type MockVerifier struct{}

func (mv *MockVerifier) Verify(ctx context.Context, service string, namespace string, traceID string) (bool, error) {
	return true, nil
}

func TestWebhookHandler(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()

	policies := []models.Policy{
		{
			ID:                "p-test",
			Service:           "payment-service",
			AlertName:         "HighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           1 * time.Second,
		},
	}

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
		},
	}

	store := storage.NewInMemoryIncidentStore()
	history := storage.NewInMemoryDecisionHistoryStore()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()
	policyEngine := policy.NewEngine(policies, m)
	cooldown := utils.NewCooldownManager()

	promMock := prometheus.NewMockClient()
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

	timeline := reliability.NewInMemoryTimelineEngine()
	reliabilityEngine := reliability.NewReliabilityEngine(store, rollbackStore, timeline, nil)

	stateMachine := state.NewStateMachine(log, m, timeline)
	recoveryPlanner := planner.NewPlanner(m)

	helmClient := helm.NewMockHelmClient()
	executorInstance := executor.NewHelmRollbackExecutor(helmClient, stateMachine, m, log)
	verifierInstance := &MockVerifier{}
	retryEngineInstance := retry.NewRetryEngine(m, log)

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
		retryEngineInstance,
		rollbackStore,
		timeline,
		reliabilityEngine,
	)

	alertParser := alert.NewParser()
	handler := NewHandler(alertParser, mgr, log)

	// Valid request body
	validJSON := `{
		"receiver": "webhook",
		"status": "firing",
		"alerts": [
			{
				"status": "firing",
				"labels": {
					"alertname": "HighErrorRate",
					"service": "payment-service",
					"severity": "critical"
				},
				"annotations": {
					"summary": "payment service errors"
				},
				"startsAt": "2026-06-27T10:00:00Z",
				"fingerprint": "fingerprint-abc"
			}
		],
		"version": "4"
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(validJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Invalid request body
	invalidJSON := `{"alerts": []}`
	reqInvalid := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(invalidJSON))
	reqInvalid.Header.Set("Content-Type", "application/json")
	wInvalid := httptest.NewRecorder()

	handler.ServeHTTP(wInvalid, reqInvalid)

	if wInvalid.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", wInvalid.Code)
	}
}
