// internal/operator/reliability/timeline.go

package reliability

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type TimelineEngine interface {
	Record(ctx context.Context, incidentID string, message string) error
	GetTimeline(ctx context.Context, incidentID string) ([]models.TimelineEvent, error)
	GetAllTimelines(ctx context.Context) (map[string][]models.TimelineEvent, error)
}

type SQLTimelineEngine struct {
	db *sql.DB
}

func NewSQLTimelineEngine(db *sql.DB) *SQLTimelineEngine {
	return &SQLTimelineEngine{db: db}
}

func (e *SQLTimelineEngine) Record(ctx context.Context, incidentID string, message string) error {
	id := fmt.Sprintf("tl-%d", time.Now().UnixNano())
	query := `INSERT INTO timeline_events (id, incident_id, timestamp, message) VALUES ($1, $2, $3, $4)`
	_, err := e.db.ExecContext(ctx, query, id, incidentID, time.Now().UTC(), message)
	return err
}

func (e *SQLTimelineEngine) GetTimeline(ctx context.Context, incidentID string) ([]models.TimelineEvent, error) {
	query := `SELECT id, incident_id, timestamp, message FROM timeline_events WHERE incident_id = $1 ORDER BY timestamp ASC`
	rows, err := e.db.QueryContext(ctx, query, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.TimelineEvent
	for rows.Next() {
		var ev models.TimelineEvent
		if err := rows.Scan(&ev.ID, &ev.IncidentID, &ev.Timestamp, &ev.Message); err == nil {
			events = append(events, ev)
		}
	}
	return events, nil
}

func (e *SQLTimelineEngine) GetAllTimelines(ctx context.Context) (map[string][]models.TimelineEvent, error) {
	query := `SELECT id, incident_id, timestamp, message FROM timeline_events ORDER BY timestamp ASC`
	rows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string][]models.TimelineEvent)
	for rows.Next() {
		var ev models.TimelineEvent
		if err := rows.Scan(&ev.ID, &ev.IncidentID, &ev.Timestamp, &ev.Message); err == nil {
			res[ev.IncidentID] = append(res[ev.IncidentID], ev)
		}
	}
	return res, nil
}

// InMemoryTimelineEngine fallback
type InMemoryTimelineEngine struct {
	mu     sync.RWMutex
	events map[string][]models.TimelineEvent
}

func NewInMemoryTimelineEngine() *InMemoryTimelineEngine {
	return &InMemoryTimelineEngine{
		events: make(map[string][]models.TimelineEvent),
	}
}

func (e *InMemoryTimelineEngine) Record(ctx context.Context, incidentID string, message string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ev := models.TimelineEvent{
		ID:         fmt.Sprintf("tl-%d", time.Now().UnixNano()),
		IncidentID: incidentID,
		Timestamp:  time.Now().UTC(),
		Message:    message,
	}
	e.events[incidentID] = append(e.events[incidentID], ev)
	return nil
}

func (e *InMemoryTimelineEngine) GetTimeline(ctx context.Context, incidentID string) ([]models.TimelineEvent, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	evs, exists := e.events[incidentID]
	if !exists {
		return []models.TimelineEvent{}, nil
	}
	return evs, nil
}

func (e *InMemoryTimelineEngine) GetAllTimelines(ctx context.Context) (map[string][]models.TimelineEvent, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	res := make(map[string][]models.TimelineEvent)
	for k, v := range e.events {
		res[k] = v
	}
	return res, nil
}
