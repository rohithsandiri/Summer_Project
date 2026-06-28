// internal/operator/state/machine.go
//
// Explicit finite state machine for managing incident lifecycle state transitions.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/reliability"
)

type StateMachine struct {
	log      *logger.Logger
	m        *metrics.OperatorMetrics
	timeline reliability.TimelineEngine
}

func NewStateMachine(log *logger.Logger, m *metrics.OperatorMetrics, timeline reliability.TimelineEngine) *StateMachine {
	return &StateMachine{log: log, m: m, timeline: timeline}
}

// Transition performs state machine transitions, validating allowed states.
func (sm *StateMachine) Transition(ctx context.Context, incident *models.Incident, toState models.State, reason string) error {
	fromState := incident.CurrentState
	if fromState == "" {
		fromState = models.StateHealthy
	}

	if !sm.isTransitionAllowed(fromState, toState) {
		err := fmt.Errorf("invalid state transition from %s to %s", fromState, toState)
		sm.log.Error(ctx, "failed state transition", logger.Fields{
			IncidentID: incident.ID,
			AlertName:  incident.AlertName,
			Service:    incident.Service,
			State:      string(fromState),
			Reason:     err.Error(),
		})
		return err
	}

	// Update incident fields
	incident.CurrentState = toState
	incident.LastUpdate = time.Now().UTC()
	incident.History = append(incident.History, models.StateTransition{
		FromState: fromState,
		ToState:   toState,
		Timestamp: incident.LastUpdate,
		Reason:    reason,
	})

	// Record in timeline
	if sm.timeline != nil {
		_ = sm.timeline.Record(ctx, incident.ID, fmt.Sprintf("State transitioned from %s to %s. Reason: %s", fromState, toState, reason))
	}

	// Increment Prometheus transitions count
	sm.m.StateTransitions.WithLabelValues(string(fromState), string(toState)).Inc()

	// Log successful transition
	sm.log.Info(ctx, "incident state transitioned", logger.Fields{
		IncidentID: incident.ID,
		AlertName:  incident.AlertName,
		Service:    incident.Service,
		State:      string(toState),
		Reason:     reason,
	}, "from_state", fromState, "to_state", toState)

	return nil
}

func (sm *StateMachine) isTransitionAllowed(from, to models.State) bool {
	if from == to {
		return true
	}

	// Transitions map
	allowed := map[models.State]map[models.State]bool{
		models.StateHealthy: {
			models.StateWarning: true,
		},
		models.StateWarning: {
			models.StateInvestigating: true,
			models.StateRecovered:     true,
		},
		models.StateInvestigating: {
			models.StateDecisionMade: true,
			models.StateRecovered:    true,
			models.StateFailed:       true,
		},
		models.StateDecisionMade: {
			models.StateRecoveryPlanned: true,
			models.StateWaiting:         true,
			models.StateFailed:          true,
		},
		models.StateRecoveryPlanned: {
			models.StateExecutingRollback: true,
			models.StateWaiting:           true,
			models.StateFailed:            true,
		},
		models.StateExecutingRollback: {
			models.StateRollbackComplete:   true,
			models.StateVerificationFailed: true,
			models.StateFailed:             true,
			models.StateRetrying:           true,
		},
		models.StateRollbackComplete: {
			models.StateVerifying:          true,
			models.StateVerificationFailed: true,
			models.StateFailed:             true,
		},
		models.StateVerifying: {
			models.StateRecovered:          true,
			models.StateVerificationFailed: true,
			models.StateFailed:             true,
		},
		models.StateVerificationFailed: {
			models.StateRetrying: true,
			models.StateFailed:   true,
		},
		models.StateRetrying: {
			models.StateExecutingRollback: true,
			models.StateFailed:            true,
		},
		models.StateWaiting: {
			models.StateExecutingRollback: true,
			models.StateRecovered:         true,
			models.StateFailed:            true,
			models.StateInvestigating:     true,
		},
		models.StateRecovered: {
			models.StateHealthy: true,
			models.StateWarning: true, // alert re-fired
		},
		models.StateFailed: {
			models.StateInvestigating: true, // retry / escalate
			models.StateWarning:       true, // alert re-fired
			models.StateHealthy:       true, // manual override
		},
	}

	targets, ok := allowed[from]
	if !ok {
		return false
	}
	return targets[to]
}
