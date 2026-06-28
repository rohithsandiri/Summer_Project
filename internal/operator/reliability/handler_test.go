// internal/operator/reliability/handler_test.go

package reliability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

func TestReliabilityHandlerRoutes(t *testing.T) {
	log := logger.New("1.0.0")
	store := storage.NewInMemoryIncidentStore()
	rollbackStore := storage.NewInMemoryRollbackHistoryStore()
	timeline := NewInMemoryTimelineEngine()
	relEngine := NewReliabilityEngine(store, rollbackStore, timeline, nil)
	handler := NewReliabilityHandler(store, timeline, relEngine, log)

	// Pre-populate an incident
	inc := &models.Incident{
		ID:           "inc-test-123",
		AlertID:      "fp-123",
		Service:      "payment-service",
		Severity:     "critical",
		Status:       "firing",
		CurrentState: models.StateInvestigating,
		StartTime:    time.Now().Add(-10 * time.Minute),
	}
	_ = store.Create(context.Background(), inc)
	_ = timeline.Record(context.Background(), "inc-test-123", "Alert Fired")

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Test /incidents
	req := httptest.NewRequest("GET", "/incidents", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var incs []*models.Incident
	if err := json.Unmarshal(rr.Body.Bytes(), &incs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(incs) != 1 || incs[0].ID != "inc-test-123" {
		t.Errorf("unexpected incidents list: %+v", incs)
	}

	// 2. Test /incidents?id=inc-test-123
	req = httptest.NewRequest("GET", "/incidents?id=inc-test-123", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var singleInc models.Incident
	if err := json.Unmarshal(rr.Body.Bytes(), &singleInc); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if singleInc.ID != "inc-test-123" {
		t.Errorf("expected inc-test-123, got %s", singleInc.ID)
	}

	// 3. Test /runbooks?incident_id=inc-test-123&format=markdown
	req = httptest.NewRequest("GET", "/runbooks?incident_id=inc-test-123&format=markdown", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	bodyStr := rr.Body.String()
	if !contains(bodyStr, "# SRE Automated Runbook") {
		t.Errorf("expected markdown header in body, got: %s", bodyStr)
	}

	// 4. Test /recommendations?service=payment-service
	req = httptest.NewRequest("GET", "/recommendations?service=payment-service", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var recs []models.OperationalRecommendation
	if err := json.Unmarshal(rr.Body.Bytes(), &recs); err != nil {
		t.Fatalf("failed to decode recommendations: %v", err)
	}
	if len(recs) == 0 || recs[0].Action != "Rollback" {
		t.Errorf("expected Rollback recommendation, got: %+v", recs)
	}

	// 5. Test /timelines?incident_id=inc-test-123
	req = httptest.NewRequest("GET", "/timelines?incident_id=inc-test-123", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var tl []models.TimelineEvent
	if err := json.Unmarshal(rr.Body.Bytes(), &tl); err != nil {
		t.Fatalf("failed to decode timelines: %v", err)
	}
	if len(tl) != 1 || tl[0].Message != "Alert Fired" {
		t.Errorf("unexpected timeline: %+v", tl)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && stringsIndex(s, substr) >= 0))
}

func stringsIndex(s, substr string) int {
	// Simple index implementation
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
