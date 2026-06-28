// internal/operator/models/progressive.go

package models

import (
	"time"
)

// RolloutState defines the state of a progressive delivery rollout.
type RolloutState string

const (
	RolloutStatePending     RolloutState = "Pending"
	RolloutStateStarting    RolloutState = "Starting"
	RolloutStateCanary      RolloutState = "Canary"
	RolloutStatePaused      RolloutState = "Paused"
	RolloutStatePromoting   RolloutState = "Promoting"
	RolloutStateCompleted   RolloutState = "Completed"
	RolloutStateAborted     RolloutState = "Aborted"
	RolloutStateRollingBack RolloutState = "RollingBack"
	RolloutStateRecovered   RolloutState = "Recovered"
	RolloutStateFailed      RolloutState = "Failed"
)

// DeploymentHistory represents a record of a progressive deployment.
type DeploymentHistory struct {
	DeploymentID       string       `json:"deployment_id"`
	Revision           int          `json:"revision"`
	HelmRelease        string       `json:"helm_release"`
	ArgoRollout        string       `json:"argo_rollout"`
	Strategy           string       `json:"strategy"` // "Canary" | "Blue-Green"
	StartTime          time.Time    `json:"start_time"`
	EndTime            time.Time    `json:"end_time,omitempty"`
	Duration           string       `json:"duration,omitempty"`
	PromotionResult    string       `json:"promotion_result"` // "Promoted" | "Aborted" | "Blocked"
	RollbackResult     string       `json:"rollback_result,omitempty"`
	VerificationResult string       `json:"verification_result,omitempty"`
	RiskScore          float64      `json:"risk_score"`
	OperatorDecision   string       `json:"operator_decision"` // "Promote" | "Abort" | "Pause" | "Block"
	CurrentState       RolloutState `json:"current_state"`
}

// DeploymentGuardConfig specifies SRE policies for blocking deployments.
type DeploymentGuardConfig struct {
	MaxAllowedBurnRate       float64 `json:"max_allowed_burn_rate"`
	MinRemainingBudget       float64 `json:"min_remaining_budget"`
	BlockOnCriticalIncidents bool    `json:"block_on_critical_incidents"`
	MinDecisionConfidence    float64 `json:"min_decision_confidence"`
}

// RiskAnalysisDetails records scoring metrics for risk calculations.
type RiskAnalysisDetails struct {
	RiskScore float64  `json:"risk_score"`
	Decision  string   `json:"decision"` // "Deploy" | "Canary" | "Pause" | "Reject"
	Reasons   []string `json:"reasons"`
}
