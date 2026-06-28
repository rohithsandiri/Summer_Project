// internal/operator/chaos/scenarios.go

package chaos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/incident"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/reliability"
	"github.com/rohithsandiri/Summer_Project/internal/operator/storage"
)

type ScenarioEngine struct {
	store             *storage.InMemoryIncidentStore
	rollbackStore     *storage.InMemoryRollbackHistoryStore
	promClient        *prometheus.MockPrometheusClient
	incidentMgr       *incident.Manager
	reliabilityEngine *reliability.ReliabilityEngine
	timeline          reliability.TimelineEngine
	log               *logger.Logger
}

func NewScenarioEngine(
	store *storage.InMemoryIncidentStore,
	rollbackStore *storage.InMemoryRollbackHistoryStore,
	promClient *prometheus.MockPrometheusClient,
	incidentMgr *incident.Manager,
	reliabilityEngine *reliability.ReliabilityEngine,
	timeline reliability.TimelineEngine,
	log *logger.Logger,
) *ScenarioEngine {
	return &ScenarioEngine{
		store:             store,
		rollbackStore:     rollbackStore,
		promClient:        promClient,
		incidentMgr:       incidentMgr,
		reliabilityEngine: reliabilityEngine,
		timeline:          timeline,
		log:               log,
	}
}

// Scenario 1: Bad Release Canary Failure
func (e *ScenarioEngine) RunScenario1(ctx context.Context) (string, error) {
	e.log.Info(ctx, "=== RUNNING SCENARIO 1: Bad Release Canary Deployment ===", logger.Fields{})

	// 1. Setup mock metric values to simulate Canary SLO violation
	e.promClient.ErrorRateValues["payment-service"] = 0.08
	e.promClient.AvailabilityValues["payment-service"] = 0.92
	e.promClient.TrafficValues["payment-service"] = 150.0

	alert := &models.Alert{
		Fingerprint: "alert-s1-canary",
		Name:        "HighErrorRate",
		Service:     "payment-service",
		Severity:    "critical",
		Status:      "firing",
		StartsAt:    time.Now().UTC().Add(-5 * time.Minute),
	}

	// 2. Alert manager pushes to operator
	err := e.incidentMgr.ProcessAlert(ctx, alert, "trace-s1-canary")
	if err != nil {
		return "", err
	}

	// Wait briefly to let async goroutine complete recovery and verification
	time.Sleep(150 * time.Millisecond)

	// 3. Resolve alert
	alert.Status = "resolved"
	alert.EndsAt = time.Now().UTC()
	err = e.incidentMgr.ProcessAlert(ctx, alert, "trace-s1-canary")
	if err != nil {
		return "", err
	}

	// 4. Retrieve incident timeline & generate report
	inc, err := e.store.GetByFingerprint(ctx, "alert-s1-canary")
	if err != nil {
		return "", err
	}

	reportPath, err := e.GenerateReport(ctx, "Scenario 1: Canary Deployment Guard Rollback", inc)
	return reportPath, err
}

// Scenario 2: Inventory Service Crash
func (e *ScenarioEngine) RunScenario2(ctx context.Context) (string, error) {
	e.log.Info(ctx, "=== RUNNING SCENARIO 2: Inventory Service Crash ===", logger.Fields{})

	e.promClient.AvailabilityValues["inventory-service"] = 0.85
	e.promClient.ErrorRateValues["inventory-service"] = 0.15
	e.promClient.TrafficValues["inventory-service"] = 80.0

	alert := &models.Alert{
		Fingerprint: "alert-s2-crash",
		Name:        "InventoryServiceDown",
		Service:     "inventory-service",
		Severity:    "critical",
		Status:      "firing",
		StartsAt:    time.Now().UTC().Add(-2 * time.Minute),
	}

	err := e.incidentMgr.ProcessAlert(ctx, alert, "trace-s2-crash")
	if err != nil {
		return "", err
	}

	time.Sleep(150 * time.Millisecond)

	alert.Status = "resolved"
	alert.EndsAt = time.Now().UTC()
	err = e.incidentMgr.ProcessAlert(ctx, alert, "trace-s2-crash")
	if err != nil {
		return "", err
	}

	inc, err := e.store.GetByFingerprint(ctx, "alert-s2-crash")
	if err != nil {
		return "", err
	}

	reportPath, err := e.GenerateReport(ctx, "Scenario 2: Inventory Crash Mitigation", inc)
	return reportPath, err
}

// Scenario 3: Database Unavailable
func (e *ScenarioEngine) RunScenario3(ctx context.Context) (string, error) {
	e.log.Info(ctx, "=== RUNNING SCENARIO 3: Database Connection Loss ===", logger.Fields{})

	e.promClient.AvailabilityValues["order-service"] = 0.70
	e.promClient.ErrorRateValues["order-service"] = 0.30
	e.promClient.TrafficValues["order-service"] = 50.0

	alert := &models.Alert{
		Fingerprint: "alert-s3-db",
		Name:        "DatabaseUnavailable",
		Service:     "order-service",
		Severity:    "critical",
		Status:      "firing",
		StartsAt:    time.Now().UTC().Add(-4 * time.Minute),
	}

	err := e.incidentMgr.ProcessAlert(ctx, alert, "trace-s3-db")
	if err != nil {
		return "", err
	}

	time.Sleep(150 * time.Millisecond)

	alert.Status = "resolved"
	alert.EndsAt = time.Now().UTC()
	err = e.incidentMgr.ProcessAlert(ctx, alert, "trace-s3-db")
	if err != nil {
		return "", err
	}

	inc, err := e.store.GetByFingerprint(ctx, "alert-s3-db")
	if err != nil {
		return "", err
	}

	reportPath, err := e.GenerateReport(ctx, "Scenario 3: Database Connection Loss Rollback", inc)
	return reportPath, err
}

// Scenario 4: High Latency & Burn Rate
func (e *ScenarioEngine) RunScenario4(ctx context.Context) (string, error) {
	e.log.Info(ctx, "=== RUNNING SCENARIO 4: High Latency & SLO Burn Rate Abort ===", logger.Fields{})

	// Simulate high latency (P95 latency spikes to 1.5 seconds)
	e.promClient.LatencyValues["payment-service"] = 1.5
	e.promClient.TrafficValues["payment-service"] = 200.0

	alert := &models.Alert{
		Fingerprint: "alert-s4-latency",
		Name:        "LatencyP95MaxViolation",
		Service:     "payment-service",
		Severity:    "warning",
		Status:      "firing",
		StartsAt:    time.Now().UTC().Add(-1 * time.Minute),
	}

	err := e.incidentMgr.ProcessAlert(ctx, alert, "trace-s4-latency")
	if err != nil {
		return "", err
	}

	time.Sleep(150 * time.Millisecond)

	alert.Status = "resolved"
	alert.EndsAt = time.Now().UTC()
	err = e.incidentMgr.ProcessAlert(ctx, alert, "trace-s4-latency")
	if err != nil {
		return "", err
	}

	inc, err := e.store.GetByFingerprint(ctx, "alert-s4-latency")
	if err != nil {
		return "", err
	}

	reportPath, err := e.GenerateReport(ctx, "Scenario 4: High Latency Burn Rate Abort", inc)
	return reportPath, err
}

// GenerateReport writes a Markdown report for the experiment to the output directory
func (e *ScenarioEngine) GenerateReport(ctx context.Context, scenarioTitle string, inc *models.Incident) (string, error) {
	runbook, err := e.reliabilityEngine.GenerateRunbook(ctx, inc.ID)
	if err != nil {
		return "", err
	}

	timeline, _ := e.timeline.GetTimeline(ctx, inc.ID)

	timelineStr := ""
	for _, t := range timeline {
		timelineStr += fmt.Sprintf("- [%s] %s\n", t.Timestamp.Format("15:04:05"), t.Message)
	}

	reportContent := fmt.Sprintf(`# Experiment Report: %s

## Scenario Configuration
- **Incident ID**: %s
- **Alert Fingerprint**: %s
- **Target Service**: %s
- **Severity Level**: %s
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
%s

## Metrics Snapshot (Before Recovery)
- **Error Rate**: %s
- **SLO Metrics**: %s

## Operator Policy Decisions
- **Decision Engine Output**: %s
- **Recommended Action**: %s
- **Helm Rollback Target Revision**: %d
- **Verification Status**: %s
- **Total Recovery Duration**: %.2f seconds

## Post-Mortem & Lessons Learned
%s
`,
		scenarioTitle,
		inc.ID,
		inc.AlertID,
		inc.Service,
		inc.Severity,
		timelineStr,
		runbook.MetricsBefore,
		runbook.MetricsAfter,
		inc.Decision,
		inc.RecoveryAction,
		inc.RollbackRevision,
		inc.VerificationResult,
		inc.DurationSeconds,
		runbook.LessonsLearned,
	)

	// Ensure output directory exists
	dir := "/Users/rohithsandiri/Downloads/project/docs/experiments"
	_ = os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("report-%s.md", inc.ID)
	path := filepath.Join(dir, filename)

	err = os.WriteFile(path, []byte(reportContent), 0644)
	if err != nil {
		return "", err
	}

	e.log.Info(ctx, "Successfully generated experiment report", logger.Fields{}, "path", path)
	return path, nil
}
