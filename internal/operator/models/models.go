// internal/operator/models/models.go
//
// Core domain models for the self-healing operator.
// Ensures strict typing instead of map[string]interface{}.

package models

import (
	"time"
)

// State represents the explicit finite state machine states.
type State string

const (
	StateHealthy            State = "Healthy"
	StateWarning            State = "Warning"
	StateInvestigating      State = "Investigating"
	StateDecisionMade       State = "DecisionMade"
	StateRecoveryPlanned    State = "RecoveryPlanned"
	StateExecutingRollback  State = "ExecutingRollback"
	StateRollbackComplete   State = "RollbackComplete"
	StateVerifying          State = "Verifying"
	StateVerificationFailed State = "VerificationFailed"
	StateRetrying           State = "Retrying"
	StateWaiting            State = "Waiting"
	StateRecovered          State = "Recovered"
	StateFailed             State = "Failed"
)

// Action represents the recommended or decided recovery actions.
type Action string

const (
	ActionIgnore          Action = "Ignore"
	ActionWait            Action = "Wait"
	ActionEscalate        Action = "Escalate"
	ActionPrepareRollback Action = "Prepare Rollback"
	ActionPrepareRestart  Action = "Prepare Restart"
	ActionPrepareScale    Action = "Prepare Scale"
)

// Alert represents an parsed and validated Alertmanager alert.
type Alert struct {
	Fingerprint  string            `json:"fingerprint"`
	Name         string            `json:"name"`
	Service      string            `json:"service"`
	Severity     string            `json:"severity"`
	Status       string            `json:"status"` // "firing" | "resolved"
	StartsAt     time.Time         `json:"starts_at"`
	EndsAt       time.Time         `json:"ends_at"`
	GeneratorURL string            `json:"generator_url"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
}

type Incident struct {
	ID                 string            `json:"id"`
	AlertID            string            `json:"alert_id"`
	Service            string            `json:"service"`
	Severity           string            `json:"severity"`
	AlertName          string            `json:"alert_name"`
	StartTime          time.Time         `json:"start_time"`
	LastUpdate         time.Time         `json:"last_update"`
	Status             string            `json:"status"` // "firing" | "resolved"
	CurrentState       State             `json:"current_state"`
	History            []StateTransition `json:"history"`
	RecoveryAttempts   int               `json:"recovery_attempts"`
	DecisionHistory    []DecisionEntry   `json:"decision_history"`
	Namespace          string            `json:"namespace"`
	Labels             map[string]string `json:"labels"`
	Annotations        map[string]string `json:"annotations"`
	RootCause          string            `json:"root_cause"`
	Decision           string            `json:"decision"`
	RecoveryAction     string            `json:"recovery_action"`
	HelmRelease        string            `json:"helm_release"`
	RollbackRevision   int               `json:"rollback_revision"`
	EndTime            time.Time         `json:"end_time"`
	DurationSeconds    float64           `json:"duration_seconds"`
	VerificationResult string            `json:"verification_result"`
	OperatorVersion    string            `json:"operator_version"`
	RecoveryConfidence float64           `json:"recovery_confidence"` // 0-100
	SLOSnapshot        string            `json:"slo_snapshot"`        // JSON summary or description
	BurnRate           float64           `json:"burn_rate"`
	ErrorBudget        float64           `json:"error_budget"`
	DeploymentRevision string            `json:"deployment_revision"`
}

// StateTransition represents a historical record of a state machine transition.
type StateTransition struct {
	FromState State     `json:"from_state"`
	ToState   State     `json:"to_state"`
	Timestamp time.Time `json:"timestamp"`
	Reason    string    `json:"reason"`
}

// DecisionEntry represents a record of a decision made for an incident.
type DecisionEntry struct {
	Timestamp               time.Time     `json:"timestamp"`
	Decision                Action        `json:"decision"`
	PolicyUsed              string        `json:"policy_used"`
	Reason                  string        `json:"reason"`
	OperatorVersion         string        `json:"operator_version"`
	RecoveryPlan            *RecoveryPlan `json:"recovery_plan,omitempty"`
	RecoveryTargetServiceID string        `json:"recovery_target_service_id,omitempty"`
}

// Policy represents a configurable rule mapping alert conditions to recommended actions.
type Policy struct {
	ID                string        `json:"id" yaml:"id"`
	Service           string        `json:"service" yaml:"service"`
	AlertName         string        `json:"alert_name" yaml:"alert_name"`
	RecommendedAction Action        `json:"recommended_action" yaml:"recommended_action"`
	CooldownDuration  time.Duration `json:"cooldown_duration" yaml:"cooldown_duration"`
	Timeout           time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries        int           `json:"max_retries" yaml:"max_retries"`
}

// RecoveryPlan represents a structured, planned set of operations to recover a service.
// This is the output of the control plane (Phase 4A) which will be consumed by the execution plane (Phase 4B).
type RecoveryPlan struct {
	ID                 string        `json:"id"`
	ServiceID          string        `json:"service_id"`
	DesiredAction      Action        `json:"desired_action"`
	Priority           int           `json:"priority"` // 1 = High, 2 = Medium, 3 = Low
	Timeout            time.Duration `json:"timeout"`
	VerificationWindow time.Duration `json:"verification_window"`
	RetryPolicy        RetryPolicy   `json:"retry_policy"`
	Cooldown           time.Duration `json:"cooldown"`
	TargetRevision     string        `json:"target_revision"` // Empty for now, populated by Helm SDK in Phase 4B
	ExecutionState     string        `json:"execution_state"` // "Pending" | "Executing" | "Completed" | "Failed"
	CreatedAt          time.Time     `json:"created_at"`
}

// RetryPolicy defines how many times a recovery action can be tried and with what backoff.
type RetryPolicy struct {
	MaxRetries int           `json:"max_retries"`
	Backoff    time.Duration `json:"backoff"`
}

// AlertmanagerPayload represents the incoming webhook JSON from Alertmanager.
type AlertmanagerPayload struct {
	Receiver          string              `json:"receiver"`
	Status            string              `json:"status"`
	Alerts            []AlertmanagerAlert `json:"alerts"`
	GroupLabels       map[string]string   `json:"groupLabels"`
	CommonLabels      map[string]string   `json:"commonLabels"`
	CommonAnnotations map[string]string   `json:"commonAnnotations"`
	ExternalURL       string              `json:"externalURL"`
	Version           string              `json:"version"`
	GroupKey          string              `json:"groupKey"`
}

// AlertmanagerAlert represents a single alert item in the Alertmanager payload.
type AlertmanagerAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// RollbackHistory records a history of completed/attempted Helm Rollbacks.
type RollbackHistory struct {
	IncidentID         string        `json:"incident_id"`
	Service            string        `json:"service"`
	Namespace          string        `json:"namespace"`
	HelmRelease        string        `json:"helm_release"`
	OldRevision        int           `json:"old_revision"`
	RollbackRevision   int           `json:"rollback_revision"`
	StartTime          time.Time     `json:"start_time"`
	FinishTime         time.Time     `json:"finish_time"`
	RecoveryDuration   time.Duration `json:"recovery_duration"`
	VerificationResult string        `json:"verification_result"` // "Success" | "Failed"
	OperatorVersion    string        `json:"operator_version"`
}

// RecoveryResult details the outcome of a recovery executor run.
type RecoveryResult struct {
	Success          bool          `json:"success"`
	OldRevision      int           `json:"old_revision"`
	RollbackRevision int           `json:"rollback_revision"`
	Message          string        `json:"message"`
	ExecutionTime    time.Duration `json:"execution_time"`
}

// SLO defines the objectives (latency, availability, error rate) for a service.
type SLO struct {
	ServiceID          string  `json:"service_id" yaml:"service_id"`
	AvailabilityTarget float64 `json:"availability_target" yaml:"availability_target"` // e.g. 0.999
	LatencyP95Max      float64 `json:"latency_p95_max" yaml:"latency_p95_max"`         // e.g. 0.300 (300ms)
	ErrorRateMax       float64 `json:"error_rate_max" yaml:"error_rate_max"`           // e.g. 0.01 (1%)
	ThroughputMin      float64 `json:"throughput_min" yaml:"throughput_min"`           // e.g. 10.0 (10 RPS)
}
