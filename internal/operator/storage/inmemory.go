// internal/operator/storage/inmemory.go
//
// Thread-safe in-memory stores implementing IncidentStore and DecisionHistoryStore interfaces.

package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type InMemoryIncidentStore struct {
	mu        sync.RWMutex
	incidents map[string]*models.Incident // key: ID
}

func NewInMemoryIncidentStore() *InMemoryIncidentStore {
	return &InMemoryIncidentStore{
		incidents: make(map[string]*models.Incident),
	}
}

func (s *InMemoryIncidentStore) Get(ctx context.Context, id string) (*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	incident, exists := s.incidents[id]
	if !exists {
		return nil, errors.New("incident not found")
	}

	// Return a copy to prevent concurrent data modification outside the store
	return s.cloneIncident(incident), nil
}

func (s *InMemoryIncidentStore) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, incident := range s.incidents {
		if incident.AlertID == fingerprint {
			return s.cloneIncident(incident), nil
		}
	}

	return nil, errors.New("incident not found")
}

func (s *InMemoryIncidentStore) Create(ctx context.Context, incident *models.Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.incidents[incident.ID]; exists {
		return errors.New("incident already exists")
	}

	s.incidents[incident.ID] = s.cloneIncident(incident)
	return nil
}

func (s *InMemoryIncidentStore) Update(ctx context.Context, incident *models.Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.incidents[incident.ID]; !exists {
		return errors.New("incident does not exist")
	}

	s.incidents[incident.ID] = s.cloneIncident(incident)
	return nil
}

func (s *InMemoryIncidentStore) ListActive(ctx context.Context) ([]*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var active []*models.Incident
	for _, incident := range s.incidents {
		if incident.Status == "firing" {
			active = append(active, s.cloneIncident(incident))
		}
	}
	return active, nil
}

func (s *InMemoryIncidentStore) ListAll(ctx context.Context) ([]*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []*models.Incident
	for _, incident := range s.incidents {
		all = append(all, s.cloneIncident(incident))
	}
	return all, nil
}

func (s *InMemoryIncidentStore) cloneIncident(in *models.Incident) *models.Incident {
	if in == nil {
		return nil
	}

	history := make([]models.StateTransition, len(in.History))
	copy(history, in.History)

	decisions := make([]models.DecisionEntry, len(in.DecisionHistory))
	for i, d := range in.DecisionHistory {
		var plan *models.RecoveryPlan
		if d.RecoveryPlan != nil {
			p := *d.RecoveryPlan
			plan = &p
		}
		decisions[i] = models.DecisionEntry{
			Timestamp:       d.Timestamp,
			Decision:        d.Decision,
			PolicyUsed:      d.PolicyUsed,
			Reason:          d.Reason,
			OperatorVersion: d.OperatorVersion,
			RecoveryPlan:    plan,
		}
	}

	return &models.Incident{
		ID:               in.ID,
		AlertID:          in.AlertID,
		Service:          in.Service,
		Severity:         in.Severity,
		AlertName:        in.AlertName,
		StartTime:        in.StartTime,
		LastUpdate:       in.LastUpdate,
		Status:           in.Status,
		CurrentState:     in.CurrentState,
		History:          history,
		RecoveryAttempts: in.RecoveryAttempts,
		DecisionHistory:  decisions,
	}
}

// ─── InMemoryDecisionHistoryStore ──────────────────────────────────────────

type InMemoryDecisionHistoryStore struct {
	mu        sync.RWMutex
	decisions map[string][]models.DecisionEntry // key: incidentID
}

func NewInMemoryDecisionHistoryStore() *InMemoryDecisionHistoryStore {
	return &InMemoryDecisionHistoryStore{
		decisions: make(map[string][]models.DecisionEntry),
	}
}

func (s *InMemoryDecisionHistoryStore) Record(ctx context.Context, entry *models.DecisionEntry, incidentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.decisions[incidentID] = append(s.decisions[incidentID], *entry)
	return nil
}

func (s *InMemoryDecisionHistoryStore) GetHistory(ctx context.Context, incidentID string) ([]models.DecisionEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, exists := s.decisions[incidentID]
	if !exists {
		return []models.DecisionEntry{}, nil
	}

	res := make([]models.DecisionEntry, len(list))
	copy(res, list)
	return res, nil
}

// ─── InMemoryRollbackHistoryStore ──────────────────────────────────────────

type InMemoryRollbackHistoryStore struct {
	mu        sync.RWMutex
	rollbacks []*models.RollbackHistory
}

func NewInMemoryRollbackHistoryStore() *InMemoryRollbackHistoryStore {
	return &InMemoryRollbackHistoryStore{
		rollbacks: make([]*models.RollbackHistory, 0),
	}
}

func (s *InMemoryRollbackHistoryStore) RecordRollback(ctx context.Context, entry *models.RollbackHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clone to avoid concurrent access issues
	clone := *entry
	s.rollbacks = append(s.rollbacks, &clone)
	return nil
}

func (s *InMemoryRollbackHistoryStore) ListRollbacks(ctx context.Context) ([]*models.RollbackHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*models.RollbackHistory, len(s.rollbacks))
	for i, r := range s.rollbacks {
		clone := *r
		res[i] = &clone
	}
	return res, nil
}
