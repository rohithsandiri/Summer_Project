// internal/operator/interfaces/interfaces.go
//
// Decoupled interfaces for all Self-Healing Platform Control Plane components.

package interfaces

import (
	"context"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

// IncidentStore defines database operations for tracking incidents.
type IncidentStore interface {
	Get(ctx context.Context, id string) (*models.Incident, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error)
	Create(ctx context.Context, incident *models.Incident) error
	Update(ctx context.Context, incident *models.Incident) error
	ListActive(ctx context.Context) ([]*models.Incident, error)
	ListAll(ctx context.Context) ([]*models.Incident, error)
}

// DecisionHistoryStore defines database operations for decision records.
type DecisionHistoryStore interface {
	Record(ctx context.Context, entry *models.DecisionEntry, incidentID string) error
	GetHistory(ctx context.Context, incidentID string) ([]models.DecisionEntry, error)
}

// PolicyEngine determines matches between incoming alerts and configured policies.
type PolicyEngine interface {
	Match(ctx context.Context, alert *models.Alert) (*models.Policy, bool)
}

// CooldownManager determines whether a recovery plan can be scheduled for an alert/service.
type CooldownManager interface {
	IsCoolingDown(ctx context.Context, service string, alertName string) (bool, time.Duration)
	RecordRecovery(ctx context.Context, service string, alertName string, duration time.Duration)
}

// StateMachine manages valid state transitions and logs progression.
type StateMachine interface {
	Transition(ctx context.Context, incident *models.Incident, toState models.State, reason string) error
}

// DecisionEngine determines the next healing action based on all runtime facts.
type DecisionEngine interface {
	MakeDecision(ctx context.Context, alert *models.Alert, policy *models.Policy, incident *models.Incident, isCoolingDown bool) (*models.DecisionEntry, error)
}

// RecoveryPlanner translates decisions into concrete plan files.
type RecoveryPlanner interface {
	Plan(ctx context.Context, incident *models.Incident, decision *models.DecisionEntry, policy *models.Policy) (*models.RecoveryPlan, error)
}

// RollbackHistoryStore defines database operations for tracking execution rollbacks.
type RollbackHistoryStore interface {
	RecordRollback(ctx context.Context, entry *models.RollbackHistory) error
	ListRollbacks(ctx context.Context) ([]*models.RollbackHistory, error)
}
