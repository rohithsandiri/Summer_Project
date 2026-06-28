// internal/operator/planner/planner.go
//
// Recovery Planner translating engine decisions into structured recovery plans.

package planner

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type Planner struct {
	m *metrics.OperatorMetrics
}

func NewPlanner(m *metrics.OperatorMetrics) *Planner {
	return &Planner{m: m}
}

// Plan converts a decided recovery Action into a structured RecoveryPlan.
func (p *Planner) Plan(ctx context.Context, incident *models.Incident, decision *models.DecisionEntry, policy *models.Policy) (*models.RecoveryPlan, error) {
	if decision.Decision == models.ActionIgnore || decision.Decision == models.ActionWait || decision.Decision == models.ActionEscalate {
		// No plan is generated for ignore/wait/escalate actions
		return nil, nil
	}

	serviceID := incident.Service
	if decision.RecoveryTargetServiceID != "" {
		serviceID = decision.RecoveryTargetServiceID
	}

	planID := fmt.Sprintf("plan-%s-%x", serviceID, p.randomBytes(4))

	// Determine priority based on action type
	priority := 2 // Medium default (e.g. Restart)
	if decision.Decision == models.ActionPrepareRollback {
		priority = 1 // High priority for rollbacks
	} else if decision.Decision == models.ActionPrepareScale {
		priority = 3 // Low priority for scaling
	}

	timeout := 5 * time.Minute
	cooldown := 5 * time.Minute
	maxRetries := 3
	if policy != nil {
		if policy.Timeout > 0 {
			timeout = policy.Timeout
		}
		if policy.CooldownDuration > 0 {
			cooldown = policy.CooldownDuration
		}
		if policy.MaxRetries > 0 {
			maxRetries = policy.MaxRetries
		}
	}

	plan := &models.RecoveryPlan{
		ID:                 planID,
		ServiceID:          serviceID,
		DesiredAction:      decision.Decision,
		Priority:           priority,
		Timeout:            timeout,
		VerificationWindow: 1 * time.Minute, // 1 minute verification check window
		RetryPolicy: models.RetryPolicy{
			MaxRetries: maxRetries,
			Backoff:    30 * time.Second, // default 30s backoff retry delay
		},
		Cooldown:       cooldown,
		TargetRevision: "", // left empty for Phase 4B
		ExecutionState: "Pending",
		CreatedAt:      time.Now().UTC(),
	}

	decision.RecoveryPlan = plan

	// Increment metric
	p.m.RecoveryPlansCreated.WithLabelValues(incident.Service, string(decision.Decision)).Inc()

	return plan, nil
}

func (p *Planner) randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
