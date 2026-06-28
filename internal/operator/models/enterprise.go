// internal/operator/models/enterprise.go

package models

import (
	"time"
)

// TimelineEvent represents a single event in the incident timeline.
type TimelineEvent struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	Timestamp  time.Time `json:"timestamp"`
	Message    string    `json:"message"`
}

// MTTRReport holds MTTR metrics for a scope.
type MTTRReport struct {
	Scope                    string        `json:"scope"` // "service" | "namespace" | "release" | "cluster"
	Target                   string        `json:"target"`
	MTTD                     time.Duration `json:"mttd"`
	MTTR                     time.Duration `json:"mttr"`
	MTBF                     time.Duration `json:"mtbf"`
	AverageRollbackTime      time.Duration `json:"average_rollback_time"`
	AverageVerificationTime  time.Duration `json:"average_verification_time"`
	AverageRecoveryTime      time.Duration `json:"average_recovery_time"`
	AverageInvestigationTime time.Duration `json:"average_investigation_time"`
}

// ReliabilityReport holds key operational reliability metrics.
type ReliabilityReport struct {
	Scope                   string  `json:"scope"`
	Target                  string  `json:"target"`
	Availability            float64 `json:"availability"`              // 0.0 - 1.0
	IncidentFrequency       float64 `json:"incident_frequency"`        // count per period
	RecoverySuccessRate     float64 `json:"recovery_success_rate"`     // percent 0-100
	RollbackSuccessRate     float64 `json:"rollback_success_rate"`     // percent 0-100
	RetrySuccessRate        float64 `json:"retry_success_rate"`        // percent 0-100
	DeploymentSuccessRate   float64 `json:"deployment_success_rate"`   // percent 0-100
	VerificationSuccessRate float64 `json:"verification_success_rate"` // percent 0-100
	SLOCompliance           float64 `json:"slo_compliance"`            // percent 0-100
	ReliabilityScore        float64 `json:"reliability_score"`         // 0-100
}

// Runbook represents an auto-generated runbook for an incident.
type Runbook struct {
	IncidentID          string          `json:"incident_id"`
	Summary             string          `json:"summary"`
	Timeline            []TimelineEvent `json:"timeline"`
	RootCause           string          `json:"root_cause"`
	MetricsBefore       string          `json:"metrics_before"`
	MetricsAfter        string          `json:"metrics_after"`
	RecoveryActions     []string        `json:"recovery_actions"`
	HelmRevision        int             `json:"helm_revision"`
	VerificationResults string          `json:"verification_results"`
	Recommendations     []string        `json:"recommendations"`
	LessonsLearned      string          `json:"lessons_learned"`
}

// KnowledgeBaseEntry records historically successful recoveries.
type KnowledgeBaseEntry struct {
	ServiceID      string `json:"service_id"`
	FailurePattern string `json:"failure_pattern"` // e.g. "High Error Rate", "Latency"
	ActionTaken    string `json:"action_taken"`    // e.g. "Helm Rollback", "Restart"
	Outcome        string `json:"outcome"`         // "Recovered" | "Failed"
}

// OperationalRecommendation represents a recommended action by the SRE operator.
type OperationalRecommendation struct {
	ServiceID  string  `json:"service_id"`
	Action     string  `json:"action"`     // "Rollback" | "Restart" | "Scale" | "Observe" | "Pause Deployment" | "Escalate"
	Confidence float64 `json:"confidence"` // 0-100
	Reason     string  `json:"reason"`
}
