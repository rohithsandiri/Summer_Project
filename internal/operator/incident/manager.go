// internal/operator/incident/manager.go
//
// Evolved Incident Manager executing control plane decisions and execution plane rollbacks.

package incident

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/executor"
	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/reliability"
	"github.com/rohithsandiri/Summer_Project/internal/operator/retry"
	"github.com/rohithsandiri/Summer_Project/internal/operator/verification"
)

type Manager struct {
	store          interfaces.IncidentStore
	history        interfaces.DecisionHistoryStore
	policyEngine   interfaces.PolicyEngine
	decisionEngine interfaces.DecisionEngine
	planner        interfaces.RecoveryPlanner
	stateMachine   interfaces.StateMachine
	cooldown       interfaces.CooldownManager
	log            *logger.Logger
	m              *metrics.OperatorMetrics

	// Phase 4B Execution Plane injections
	executor      executor.RecoveryExecutor
	verifier      verification.VerificationEngine
	retryEngine   *retry.RetryEngine
	rollbackStore interfaces.RollbackHistoryStore

	// Phase 7 Enterprise injections
	timeline    reliability.TimelineEngine
	reliability *reliability.ReliabilityEngine

	// Leader election fields
	isLeader bool
	leaderMu sync.RWMutex
}

func (mgr *Manager) SetLeader(leader bool) {
	mgr.leaderMu.Lock()
	defer mgr.leaderMu.Unlock()
	mgr.isLeader = leader
}

func (mgr *Manager) IsLeader() bool {
	mgr.leaderMu.RLock()
	defer mgr.leaderMu.RUnlock()
	return mgr.isLeader
}

func NewManager(
	store interfaces.IncidentStore,
	history interfaces.DecisionHistoryStore,
	policyEngine interfaces.PolicyEngine,
	decisionEngine interfaces.DecisionEngine,
	planner interfaces.RecoveryPlanner,
	stateMachine interfaces.StateMachine,
	cooldown interfaces.CooldownManager,
	log *logger.Logger,
	m *metrics.OperatorMetrics,
	executor executor.RecoveryExecutor,
	verifier verification.VerificationEngine,
	retryEngine *retry.RetryEngine,
	rollbackStore interfaces.RollbackHistoryStore,
	timeline reliability.TimelineEngine,
	reliability *reliability.ReliabilityEngine,
) *Manager {
	return &Manager{
		store:          store,
		history:        history,
		policyEngine:   policyEngine,
		decisionEngine: decisionEngine,
		planner:        planner,
		stateMachine:   stateMachine,
		cooldown:       cooldown,
		log:            log,
		m:              m,
		executor:       executor,
		verifier:       verifier,
		retryEngine:    retryEngine,
		rollbackStore:  rollbackStore,
		timeline:       timeline,
		reliability:    reliability,
		isLeader:       true,
	}
}

// ProcessAlert ingests an alert, creates/updates an incident, and drives decision/planning.
func (mgr *Manager) ProcessAlert(ctx context.Context, alert *models.Alert, traceID string) error {
	f := logger.Fields{
		TraceID:   traceID,
		AlertName: alert.Name,
		Service:   alert.Service,
	}

	if !mgr.IsLeader() {
		mgr.log.Info(ctx, "skipping alert processing: this replica is not the active leader", f)
		return nil
	}

	mgr.log.Info(ctx, "processing alert", f, "fingerprint", alert.Fingerprint, "status", alert.Status)

	// Increment metrics
	mgr.m.AlertsReceived.Inc()

	// 1. Get or Create Incident
	incident, err := mgr.store.GetByFingerprint(ctx, alert.Fingerprint)
	if err != nil {
		// Create new incident for firing alerts
		if alert.Status == "firing" {
			incidentID := fmt.Sprintf("inc-%x", mgr.randomBytes(4))
			incident = &models.Incident{
				ID:               incidentID,
				AlertID:          alert.Fingerprint,
				Service:          alert.Service,
				Severity:         alert.Severity,
				AlertName:        alert.Name,
				StartTime:        alert.StartsAt,
				LastUpdate:       time.Now().UTC(),
				Status:           alert.Status,
				CurrentState:     models.StateHealthy, // initial state
				History:          []models.StateTransition{},
				RecoveryAttempts: 0,
				DecisionHistory:  []models.DecisionEntry{},
			}

			if err := mgr.store.Create(ctx, incident); err != nil {
				return fmt.Errorf("failed to create incident: %w", err)
			}
			if mgr.timeline != nil {
				_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Alert Fired: %s on service %s (Severity: %s)", alert.Name, alert.Service, alert.Severity))
			}
			mgr.m.IncidentsCreated.Inc()
			mgr.m.ActiveIncidents.Inc()
			mgr.log.Info(ctx, "created new incident", f, "incident_id", incident.ID)
		} else {
			// Alert resolved but no active incident found — ignore
			mgr.log.Info(ctx, "skipping resolved alert with no active incident", f, "fingerprint", alert.Fingerprint)
			return nil
		}
	}

	f.IncidentID = incident.ID

	// 2. Handle Resolved Alert
	if alert.Status == "resolved" {
		if incident.Status != "resolved" {
			incident.Status = "resolved"
			incident.EndTime = time.Now().UTC()
			if incident.StartTime.Before(incident.EndTime) {
				incident.DurationSeconds = incident.EndTime.Sub(incident.StartTime).Seconds()
			}
			if mgr.timeline != nil {
				_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Alert Resolved: %s", alert.Name))
			}
			_ = mgr.stateMachine.Transition(ctx, incident, models.StateRecovered, "Alert resolved by Alertmanager")
			_ = mgr.stateMachine.Transition(ctx, incident, models.StateHealthy, "Incident resolved and archived")
			_ = mgr.store.Update(ctx, incident)
			mgr.m.ActiveIncidents.Dec()
		}
		return nil
	}

	// 3. Firing alert — transition from Healthy to Warning
	if incident.CurrentState == models.StateHealthy {
		if err := mgr.stateMachine.Transition(ctx, incident, models.StateWarning, "Alert started firing"); err != nil {
			return err
		}
	}

	// 4. Move to Investigating
	if err := mgr.stateMachine.Transition(ctx, incident, models.StateInvestigating, "Operator evaluating alert policies"); err != nil {
		return err
	}

	// 5. Evaluate policies and cooldown status
	policy, _ := mgr.policyEngine.Match(ctx, alert)
	isCoolingDown, cooldownRemaining := mgr.cooldown.IsCoolingDown(ctx, alert.Service, alert.Name)

	if isCoolingDown {
		mgr.log.Warn(ctx, "service is cooling down", f, "remaining_cooldown", cooldownRemaining.String())
	}

	// 6. Invoke Decision Engine
	decision, err := mgr.decisionEngine.MakeDecision(ctx, alert, policy, incident, isCoolingDown)
	if err != nil {
		return fmt.Errorf("decision engine error: %w", err)
	}

	f.Policy = decision.PolicyUsed
	f.Decision = string(decision.Decision)
	f.Reason = decision.Reason

	mgr.log.Info(ctx, "decision completed", f)
	if mgr.timeline != nil {
		_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Decision Generated: Action=%s, Policy=%s, Reason=%s", decision.Decision, decision.PolicyUsed, decision.Reason))
	}

	// Move to DecisionMade
	if err := mgr.stateMachine.Transition(ctx, incident, models.StateDecisionMade, fmt.Sprintf("Decision completed: %s", decision.Decision)); err != nil {
		return err
	}

	// 7. Recovery Planning
	var plan *models.RecoveryPlan
	if decision.Decision != models.ActionIgnore && decision.Decision != models.ActionWait && decision.Decision != models.ActionEscalate {
		// Generate concrete recovery plan
		plan, err = mgr.planner.Plan(ctx, incident, decision, policy)
		if err != nil {
			return fmt.Errorf("recovery planner error: %w", err)
		}

		if plan != nil {
			incident.RecoveryAttempts++
			// Record cooldown window
			mgr.cooldown.RecordRecovery(ctx, alert.Service, alert.Name, plan.Cooldown)
			if mgr.timeline != nil {
				_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Recovery Plan Created: ID=%s, Action=%s, Cooldown=%s", plan.ID, plan.DesiredAction, plan.Cooldown.String()))
			}

			// Transition state to RecoveryPlanned
			if err := mgr.stateMachine.Transition(ctx, incident, models.StateRecoveryPlanned, fmt.Sprintf("Recovery plan created: %s", plan.ID)); err != nil {
				return err
			}

			// Trigger recovery execution plane in a separate background context
			go mgr.runRecoveryPipeline(context.Background(), incident, plan, policy, traceID)
		}
	}

	// 8. Record Decision and Update Incident
	incident.DecisionHistory = append(incident.DecisionHistory, *decision)
	incident.LastUpdate = time.Now().UTC()

	// If no action is taken, transition directly to Waiting or Failed
	if plan == nil {
		if decision.Decision != models.ActionEscalate && decision.Decision != models.ActionIgnore {
			if err := mgr.stateMachine.Transition(ctx, incident, models.StateWaiting, "Waiting for recovery execution context"); err != nil {
				return err
			}
		} else if decision.Decision == models.ActionEscalate {
			if err := mgr.stateMachine.Transition(ctx, incident, models.StateFailed, "Recovery failed or max retries exceeded. Manual intervention needed."); err != nil {
				return err
			}
		}
	}

	// Persist to store
	if err := mgr.store.Update(ctx, incident); err != nil {
		return fmt.Errorf("failed to persist incident update: %w", err)
	}

	// Persist decision history entry
	if err := mgr.history.Record(ctx, decision, incident.ID); err != nil {
		mgr.log.Error(ctx, "failed to record decision history", f)
	}

	return nil
}

// runRecoveryPipeline coordinates execution and verification loop in Phase 4B.
func (mgr *Manager) runRecoveryPipeline(
	ctx context.Context,
	incident *models.Incident,
	plan *models.RecoveryPlan,
	policy *models.Policy,
	traceID string,
) {
	startTime := time.Now().UTC()
	_, k8sNamespace := executor.ResolveReleaseAndNamespace(plan.ServiceID)

	f := logger.Fields{
		TraceID:    traceID,
		IncidentID: incident.ID,
		Service:    plan.ServiceID,
		Policy:     policy.ID,
		Decision:   string(plan.DesiredAction),
	}

	mgr.log.Info(ctx, "starting rollback execution pipeline", f)

	maxAttempts := plan.RetryPolicy.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var executionResult *models.RecoveryResult
	var lastErr error

	// Run attempts loop to satisfy transition specifications
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && lastErr != nil {
			// Transition to Retrying state
			_ = mgr.stateMachine.Transition(ctx, incident, models.StateRetrying, fmt.Sprintf("Retrying rollback attempt %d after error: %s", attempt, lastErr.Error()))
			_ = mgr.store.Update(ctx, incident)

			// Sleep exponential backoff
			backoff := mgr.retryEngine.CalculateBackoff(attempt-1, plan.RetryPolicy.Backoff)
			mgr.log.Info(ctx, "retry cooldown sleep active", f, "attempt", attempt, "backoff", backoff.String())

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
		}

		if mgr.timeline != nil {
			_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Executing Rollback (Attempt %d/%d) on service %s", attempt, maxAttempts, plan.ServiceID))
		}

		// Execute Rollback
		res, err := mgr.executor.Execute(ctx, plan, incident, traceID)
		executionResult = res
		lastErr = err
		_ = mgr.store.Update(ctx, incident)

		if err != nil {
			mgr.log.Error(ctx, "rollback execution failed", f, "attempt", attempt, "error", err.Error())
			if mgr.timeline != nil {
				_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Rollback execution failed on attempt %d: %s", attempt, err.Error()))
			}
			if !mgr.retryEngine.IsTransient(err) {
				mgr.log.Error(ctx, "non-transient error, aborting recovery", f)
				break
			}
			continue
		}

		if mgr.timeline != nil && res != nil {
			_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Rollback execution succeeded. Old Revision: %d, Rollback Revision: %d", res.OldRevision, res.RollbackRevision))
		}

		// Rollback completed successfully -> Verifying
		_ = mgr.stateMachine.Transition(ctx, incident, models.StateVerifying, "Validating system health")
		_ = mgr.store.Update(ctx, incident)

		if mgr.timeline != nil {
			_ = mgr.timeline.Record(ctx, incident.ID, "System health verification check started")
		}

		// Run Verification loop
		verified := false

		// Run first check immediately!
		ok, vErr := mgr.verifier.Verify(ctx, plan.ServiceID, k8sNamespace, traceID)
		if vErr != nil {
			mgr.log.Warn(ctx, "verification checks returned error", f, "error", vErr.Error())
		}
		if ok {
			verified = true
		} else {
			verifyTimeout := time.After(plan.Timeout)
			verifyTicker := time.NewTicker(2 * time.Second)
			defer verifyTicker.Stop()

		VerifyLoop:
			for {
				select {
				case <-verifyTimeout:
					lastErr = fmt.Errorf("verification timed out after %s", plan.Timeout.String())
					break VerifyLoop
				case <-verifyTicker.C:
					ok, vErr = mgr.verifier.Verify(ctx, plan.ServiceID, k8sNamespace, traceID)
					if vErr != nil {
						mgr.log.Warn(ctx, "verification checks returned error", f, "error", vErr.Error())
					}
					if ok {
						verified = true
						break VerifyLoop
					}
				case <-ctx.Done():
					return
				}
			}
		}

		if verified {
			// Recovery Succeeded!
			totalDuration := time.Since(startTime)

			if mgr.timeline != nil {
				_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Verification Successful. System health restored in %s.", totalDuration.String()))
			}

			// Record rollback history
			historyEntry := &models.RollbackHistory{
				IncidentID:         incident.ID,
				Service:            plan.ServiceID,
				Namespace:          k8sNamespace,
				HelmRelease:        plan.ServiceID,
				OldRevision:        executionResult.OldRevision,
				RollbackRevision:   executionResult.RollbackRevision,
				StartTime:          startTime,
				FinishTime:         time.Now().UTC(),
				RecoveryDuration:   totalDuration,
				VerificationResult: "Success",
				OperatorVersion:    executionResult.Message,
			}

			// Populate incident metadata fields
			incident.EndTime = time.Now().UTC()
			incident.DurationSeconds = totalDuration.Seconds()
			incident.RootCause = fmt.Sprintf("High failure rate/latency SLO violation detected on %s.", plan.ServiceID)
			incident.Decision = string(plan.DesiredAction)
			incident.RecoveryAction = "Helm Rollback"
			incident.HelmRelease = plan.ServiceID
			incident.RollbackRevision = executionResult.RollbackRevision
			incident.VerificationResult = "Success"
			incident.OperatorVersion = "1.0.0"
			incident.RecoveryConfidence = 95.0
			incident.Namespace = k8sNamespace

			if mgr.reliability != nil {
				_ = mgr.reliability.SaveRecovery(ctx, models.KnowledgeBaseEntry{
					ServiceID:      plan.ServiceID,
					FailurePattern: incident.AlertName,
					ActionTaken:    "Helm Rollback",
					Outcome:        "Recovered",
				})
			}

			_ = mgr.rollbackStore.RecordRollback(ctx, historyEntry)
			_ = mgr.stateMachine.Transition(ctx, incident, models.StateRecovered, "System verified healthy")
			_ = mgr.stateMachine.Transition(ctx, incident, models.StateHealthy, "Incident resolved and archived")

			// Update metrics
			mgr.m.RecoveryDurationSeconds.WithLabelValues(plan.ServiceID).Observe(totalDuration.Seconds())
			mgr.m.LastSuccessfulRollbackTime.WithLabelValues(plan.ServiceID).Set(float64(time.Now().Unix()))

			mgr.m.ActiveIncidents.Dec()
			incident.Status = "resolved"
			_ = mgr.store.Update(ctx, incident)
			return
		}

		// Verification failed
		mgr.log.Warn(ctx, "verification checks failed", f, "attempt", attempt)
		if mgr.timeline != nil {
			_ = mgr.timeline.Record(ctx, incident.ID, fmt.Sprintf("Verification failed on attempt %d.", attempt))
		}
		_ = mgr.stateMachine.Transition(ctx, incident, models.StateVerificationFailed, "System failed readiness checks")
		_ = mgr.store.Update(ctx, incident)
	}

	// Recovery exhausted or failed
	mgr.log.Error(ctx, "all recovery execution and verification attempts exhausted", f)
	if mgr.timeline != nil {
		_ = mgr.timeline.Record(ctx, incident.ID, "All recovery attempts exhausted. System remains unhealthy.")
	}

	incident.EndTime = time.Now().UTC()
	incident.DurationSeconds = incident.EndTime.Sub(incident.StartTime).Seconds()
	incident.VerificationResult = "Failed"

	_ = mgr.stateMachine.Transition(ctx, incident, models.StateFailed, "Recovery failed or max retries exceeded. Manual intervention needed.")

	// Record fail history
	historyEntry := &models.RollbackHistory{
		IncidentID:         incident.ID,
		Service:            plan.ServiceID,
		Namespace:          k8sNamespace,
		HelmRelease:        plan.ServiceID,
		StartTime:          startTime,
		FinishTime:         time.Now().UTC(),
		RecoveryDuration:   time.Since(startTime),
		VerificationResult: "Failed",
	}
	if executionResult != nil {
		historyEntry.OldRevision = executionResult.OldRevision
		historyEntry.RollbackRevision = executionResult.RollbackRevision
	}
	_ = mgr.rollbackStore.RecordRollback(ctx, historyEntry)

	_ = mgr.store.Update(ctx, incident)
}

func (mgr *Manager) randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
