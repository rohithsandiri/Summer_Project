// internal/operator/storage/postgres.go

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type PostgresIncidentStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func ConnectPostgres() (*sql.DB, error) {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		return nil, errors.New("POSTGRES_HOST is empty")
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("POSTGRES_PASSWORD")
	dbName := os.Getenv("POSTGRES_DB")
	if dbName == "" {
		dbName = "operator"
	}
	sslmode := os.Getenv("POSTGRES_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbName, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Validate connectivity with quick ping timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func NewPostgresIncidentStore(db *sql.DB) (*PostgresIncidentStore, error) {
	s := &PostgresIncidentStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresIncidentStore) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS incidents (
			id VARCHAR(128) PRIMARY KEY,
			alert_id VARCHAR(128),
			service VARCHAR(128),
			severity VARCHAR(64),
			alert_name VARCHAR(128),
			start_time TIMESTAMP WITH TIME ZONE,
			last_update TIMESTAMP WITH TIME ZONE,
			status VARCHAR(64),
			current_state VARCHAR(64),
			history TEXT,
			recovery_attempts INT DEFAULT 0,
			decision_history TEXT,
			namespace VARCHAR(128),
			labels TEXT,
			annotations TEXT,
			root_cause TEXT,
			decision TEXT,
			recovery_action TEXT,
			helm_release VARCHAR(128),
			rollback_revision INT DEFAULT 0,
			end_time TIMESTAMP WITH TIME ZONE,
			duration_seconds DOUBLE PRECISION DEFAULT 0.0,
			verification_result TEXT,
			operator_version VARCHAR(64),
			recovery_confidence DOUBLE PRECISION DEFAULT 0.0,
			slo_snapshot TEXT,
			burn_rate DOUBLE PRECISION DEFAULT 0.0,
			error_budget DOUBLE PRECISION DEFAULT 0.0,
			deployment_revision VARCHAR(128)
		);`,
		`CREATE TABLE IF NOT EXISTS timeline_events (
			id VARCHAR(128) PRIMARY KEY,
			incident_id VARCHAR(128) NOT NULL,
			timestamp TIMESTAMP WITH TIME ZONE,
			message TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS rollback_history (
			incident_id VARCHAR(128),
			service VARCHAR(128),
			namespace VARCHAR(128),
			helm_release VARCHAR(128),
			old_revision INT,
			rollback_revision INT,
			start_time TIMESTAMP WITH TIME ZONE,
			finish_time TIMESTAMP WITH TIME ZONE,
			recovery_duration_ns BIGINT,
			verification_result VARCHAR(64),
			operator_version VARCHAR(64)
		);`,
		`CREATE TABLE IF NOT EXISTS decision_history (
			incident_id VARCHAR(128),
			timestamp TIMESTAMP WITH TIME ZONE,
			decision VARCHAR(128),
			policy_used VARCHAR(128),
			reason TEXT,
			operator_version VARCHAR(64),
			recovery_plan_json TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS knowledge_base (
			service VARCHAR(128),
			failure_pattern VARCHAR(128),
			action_taken VARCHAR(128),
			outcome VARCHAR(64)
		);`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed migration: %w", err)
		}
	}
	return nil
}

func (s *PostgresIncidentStore) Get(ctx context.Context, id string) (*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var inc models.Incident
	var historyJSON, decisionJSON, labelsJSON, annotationsJSON string
	var endTime sql.NullTime

	query := `SELECT id, alert_id, service, severity, alert_name, start_time, last_update, status,
	                 current_state, history, recovery_attempts, decision_history, namespace, labels,
	                 annotations, root_cause, decision, recovery_action, helm_release, rollback_revision,
	                 end_time, duration_seconds, verification_result, operator_version, recovery_confidence,
	                 slo_snapshot, burn_rate, error_budget, deployment_revision
	          FROM incidents WHERE id = $1`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&inc.ID, &inc.AlertID, &inc.Service, &inc.Severity, &inc.AlertName, &inc.StartTime, &inc.LastUpdate, &inc.Status,
		&inc.CurrentState, &historyJSON, &inc.RecoveryAttempts, &decisionJSON, &inc.Namespace, &labelsJSON,
		&annotationsJSON, &inc.RootCause, &inc.Decision, &inc.RecoveryAction, &inc.HelmRelease, &inc.RollbackRevision,
		&endTime, &inc.DurationSeconds, &inc.VerificationResult, &inc.OperatorVersion, &inc.RecoveryConfidence,
		&inc.SLOSnapshot, &inc.BurnRate, &inc.ErrorBudget, &inc.DeploymentRevision,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("incident not found")
	} else if err != nil {
		return nil, err
	}

	if endTime.Valid {
		inc.EndTime = endTime.Time
	}

	_ = json.Unmarshal([]byte(historyJSON), &inc.History)
	_ = json.Unmarshal([]byte(decisionJSON), &inc.DecisionHistory)
	_ = json.Unmarshal([]byte(labelsJSON), &inc.Labels)
	_ = json.Unmarshal([]byte(annotationsJSON), &inc.Annotations)

	return &inc, nil
}

func (s *PostgresIncidentStore) GetByFingerprint(ctx context.Context, fingerprint string) (*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var inc models.Incident
	var historyJSON, decisionJSON, labelsJSON, annotationsJSON string
	var endTime sql.NullTime

	query := `SELECT id, alert_id, service, severity, alert_name, start_time, last_update, status,
	                 current_state, history, recovery_attempts, decision_history, namespace, labels,
	                 annotations, root_cause, decision, recovery_action, helm_release, rollback_revision,
	                 end_time, duration_seconds, verification_result, operator_version, recovery_confidence,
	                 slo_snapshot, burn_rate, error_budget, deployment_revision
	          FROM incidents WHERE alert_id = $1`

	err := s.db.QueryRowContext(ctx, query, fingerprint).Scan(
		&inc.ID, &inc.AlertID, &inc.Service, &inc.Severity, &inc.AlertName, &inc.StartTime, &inc.LastUpdate, &inc.Status,
		&inc.CurrentState, &historyJSON, &inc.RecoveryAttempts, &decisionJSON, &inc.Namespace, &labelsJSON,
		&annotationsJSON, &inc.RootCause, &inc.Decision, &inc.RecoveryAction, &inc.HelmRelease, &inc.RollbackRevision,
		&endTime, &inc.DurationSeconds, &inc.VerificationResult, &inc.OperatorVersion, &inc.RecoveryConfidence,
		&inc.SLOSnapshot, &inc.BurnRate, &inc.ErrorBudget, &inc.DeploymentRevision,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("incident not found")
	} else if err != nil {
		return nil, err
	}

	if endTime.Valid {
		inc.EndTime = endTime.Time
	}

	_ = json.Unmarshal([]byte(historyJSON), &inc.History)
	_ = json.Unmarshal([]byte(decisionJSON), &inc.DecisionHistory)
	_ = json.Unmarshal([]byte(labelsJSON), &inc.Labels)
	_ = json.Unmarshal([]byte(annotationsJSON), &inc.Annotations)

	return &inc, nil
}

func (s *PostgresIncidentStore) Create(ctx context.Context, inc *models.Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	historyBytes, _ := json.Marshal(inc.History)
	decisionBytes, _ := json.Marshal(inc.DecisionHistory)
	labelsBytes, _ := json.Marshal(inc.Labels)
	annotationsBytes, _ := json.Marshal(inc.Annotations)

	query := `INSERT INTO incidents (
		id, alert_id, service, severity, alert_name, start_time, last_update, status,
		current_state, history, recovery_attempts, decision_history, namespace, labels,
		annotations, root_cause, decision, recovery_action, helm_release, rollback_revision,
		end_time, duration_seconds, verification_result, operator_version, recovery_confidence,
		slo_snapshot, burn_rate, error_budget, deployment_revision
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)`

	var endTime sql.NullTime
	if !inc.EndTime.IsZero() {
		endTime.Time = inc.EndTime
		endTime.Valid = true
	}

	_, err := s.db.ExecContext(ctx, query,
		inc.ID, inc.AlertID, inc.Service, inc.Severity, inc.AlertName, inc.StartTime, inc.LastUpdate, inc.Status,
		inc.CurrentState, string(historyBytes), inc.RecoveryAttempts, string(decisionBytes), inc.Namespace, string(labelsBytes),
		string(annotationsBytes), inc.RootCause, inc.Decision, inc.RecoveryAction, inc.HelmRelease, inc.RollbackRevision,
		endTime, inc.DurationSeconds, inc.VerificationResult, inc.OperatorVersion, inc.RecoveryConfidence,
		inc.SLOSnapshot, inc.BurnRate, inc.ErrorBudget, inc.DeploymentRevision,
	)
	return err
}

func (s *PostgresIncidentStore) Update(ctx context.Context, inc *models.Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	historyBytes, _ := json.Marshal(inc.History)
	decisionBytes, _ := json.Marshal(inc.DecisionHistory)
	labelsBytes, _ := json.Marshal(inc.Labels)
	annotationsBytes, _ := json.Marshal(inc.Annotations)

	query := `UPDATE incidents SET
		alert_id=$2, service=$3, severity=$4, alert_name=$5, start_time=$6, last_update=$7, status=$8,
		current_state=$9, history=$10, recovery_attempts=$11, decision_history=$12, namespace=$13, labels=$14,
		annotations=$15, root_cause=$16, decision=$17, recovery_action=$18, helm_release=$19, rollback_revision=$20,
		end_time=$21, duration_seconds=$22, verification_result=$23, operator_version=$24, recovery_confidence=$25,
		slo_snapshot=$26, burn_rate=$27, error_budget=$28, deployment_revision=$29
	WHERE id=$1`

	var endTime sql.NullTime
	if !inc.EndTime.IsZero() {
		endTime.Time = inc.EndTime
		endTime.Valid = true
	}

	_, err := s.db.ExecContext(ctx, query,
		inc.ID, inc.AlertID, inc.Service, inc.Severity, inc.AlertName, inc.StartTime, inc.LastUpdate, inc.Status,
		inc.CurrentState, string(historyBytes), inc.RecoveryAttempts, string(decisionBytes), inc.Namespace, string(labelsBytes),
		string(annotationsBytes), inc.RootCause, inc.Decision, inc.RecoveryAction, inc.HelmRelease, inc.RollbackRevision,
		endTime, inc.DurationSeconds, inc.VerificationResult, inc.OperatorVersion, inc.RecoveryConfidence,
		inc.SLOSnapshot, inc.BurnRate, inc.ErrorBudget, inc.DeploymentRevision,
	)
	return err
}

func (s *PostgresIncidentStore) ListActive(ctx context.Context) ([]*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, alert_id, service, severity, alert_name, start_time, last_update, status,
	                 current_state, history, recovery_attempts, decision_history, namespace, labels,
	                 annotations, root_cause, decision, recovery_action, helm_release, rollback_revision,
	                 end_time, duration_seconds, verification_result, operator_version, recovery_confidence,
	                 slo_snapshot, burn_rate, error_budget, deployment_revision
	          FROM incidents WHERE status = 'firing'`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Incident
	for rows.Next() {
		var inc models.Incident
		var historyJSON, decisionJSON, labelsJSON, annotationsJSON string
		var endTime sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.AlertID, &inc.Service, &inc.Severity, &inc.AlertName, &inc.StartTime, &inc.LastUpdate, &inc.Status,
			&inc.CurrentState, &historyJSON, &inc.RecoveryAttempts, &decisionJSON, &inc.Namespace, &labelsJSON,
			&annotationsJSON, &inc.RootCause, &inc.Decision, &inc.RecoveryAction, &inc.HelmRelease, &inc.RollbackRevision,
			&endTime, &inc.DurationSeconds, &inc.VerificationResult, &inc.OperatorVersion, &inc.RecoveryConfidence,
			&inc.SLOSnapshot, &inc.BurnRate, &inc.ErrorBudget, &inc.DeploymentRevision,
		)
		if err != nil {
			return nil, err
		}

		if endTime.Valid {
			inc.EndTime = endTime.Time
		}

		_ = json.Unmarshal([]byte(historyJSON), &inc.History)
		_ = json.Unmarshal([]byte(decisionJSON), &inc.DecisionHistory)
		_ = json.Unmarshal([]byte(labelsJSON), &inc.Labels)
		_ = json.Unmarshal([]byte(annotationsJSON), &inc.Annotations)

		list = append(list, &inc)
	}

	return list, nil
}

func (s *PostgresIncidentStore) ListAll(ctx context.Context) ([]*models.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, alert_id, service, severity, alert_name, start_time, last_update, status,
	                 current_state, history, recovery_attempts, decision_history, namespace, labels,
	                 annotations, root_cause, decision, recovery_action, helm_release, rollback_revision,
	                 end_time, duration_seconds, verification_result, operator_version, recovery_confidence,
	                 slo_snapshot, burn_rate, error_budget, deployment_revision
	          FROM incidents ORDER BY start_time DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.Incident
	for rows.Next() {
		var inc models.Incident
		var historyJSON, decisionJSON, labelsJSON, annotationsJSON string
		var endTime sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.AlertID, &inc.Service, &inc.Severity, &inc.AlertName, &inc.StartTime, &inc.LastUpdate, &inc.Status,
			&inc.CurrentState, &historyJSON, &inc.RecoveryAttempts, &decisionJSON, &inc.Namespace, &labelsJSON,
			&annotationsJSON, &inc.RootCause, &inc.Decision, &inc.RecoveryAction, &inc.HelmRelease, &inc.RollbackRevision,
			&endTime, &inc.DurationSeconds, &inc.VerificationResult, &inc.OperatorVersion, &inc.RecoveryConfidence,
			&inc.SLOSnapshot, &inc.BurnRate, &inc.ErrorBudget, &inc.DeploymentRevision,
		)
		if err != nil {
			return nil, err
		}

		if endTime.Valid {
			inc.EndTime = endTime.Time
		}

		_ = json.Unmarshal([]byte(historyJSON), &inc.History)
		_ = json.Unmarshal([]byte(decisionJSON), &inc.DecisionHistory)
		_ = json.Unmarshal([]byte(labelsJSON), &inc.Labels)
		_ = json.Unmarshal([]byte(annotationsJSON), &inc.Annotations)

		list = append(list, &inc)
	}

	return list, nil
}

// ─── PostgresDecisionHistoryStore ─────────────────────────────────────────

type PostgresDecisionHistoryStore struct {
	db *sql.DB
}

func NewPostgresDecisionHistoryStore(db *sql.DB) *PostgresDecisionHistoryStore {
	return &PostgresDecisionHistoryStore{db: db}
}

func (s *PostgresDecisionHistoryStore) Record(ctx context.Context, entry *models.DecisionEntry, incidentID string) error {
	var planJSON string
	if entry.RecoveryPlan != nil {
		planBytes, _ := json.Marshal(entry.RecoveryPlan)
		planJSON = string(planBytes)
	}

	query := `INSERT INTO decision_history (incident_id, timestamp, decision, policy_used, reason, operator_version, recovery_plan_json)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := s.db.ExecContext(ctx, query,
		incidentID, entry.Timestamp, string(entry.Decision), entry.PolicyUsed, entry.Reason, entry.OperatorVersion, planJSON,
	)
	return err
}

func (s *PostgresDecisionHistoryStore) GetHistory(ctx context.Context, incidentID string) ([]models.DecisionEntry, error) {
	query := `SELECT timestamp, decision, policy_used, reason, operator_version, recovery_plan_json
	          FROM decision_history WHERE incident_id = $1 ORDER BY timestamp ASC`

	rows, err := s.db.QueryContext(ctx, query, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.DecisionEntry
	for rows.Next() {
		var entry models.DecisionEntry
		var decisionStr string
		var planJSON sql.NullString

		err := rows.Scan(&entry.Timestamp, &decisionStr, &entry.PolicyUsed, &entry.Reason, &entry.OperatorVersion, &planJSON)
		if err != nil {
			return nil, err
		}

		entry.Decision = models.Action(decisionStr)
		if planJSON.Valid && planJSON.String != "" {
			var plan models.RecoveryPlan
			if err := json.Unmarshal([]byte(planJSON.String), &plan); err == nil {
				entry.RecoveryPlan = &plan
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ─── PostgresRollbackHistoryStore ─────────────────────────────────────────

type PostgresRollbackHistoryStore struct {
	db *sql.DB
}

func NewPostgresRollbackHistoryStore(db *sql.DB) *PostgresRollbackHistoryStore {
	return &PostgresRollbackHistoryStore{db: db}
}

func (s *PostgresRollbackHistoryStore) RecordRollback(ctx context.Context, entry *models.RollbackHistory) error {
	query := `INSERT INTO rollback_history (incident_id, service, namespace, helm_release, old_revision, rollback_revision, start_time, finish_time, recovery_duration_ns, verification_result, operator_version)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := s.db.ExecContext(ctx, query,
		entry.IncidentID, entry.Service, entry.Namespace, entry.HelmRelease, entry.OldRevision, entry.RollbackRevision,
		entry.StartTime, entry.FinishTime, entry.RecoveryDuration.Nanoseconds(), entry.VerificationResult, entry.OperatorVersion,
	)
	return err
}

func (s *PostgresRollbackHistoryStore) ListRollbacks(ctx context.Context) ([]*models.RollbackHistory, error) {
	query := `SELECT incident_id, service, namespace, helm_release, old_revision, rollback_revision, start_time, finish_time, recovery_duration_ns, verification_result, operator_version
	          FROM rollback_history ORDER BY start_time DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.RollbackHistory
	for rows.Next() {
		var entry models.RollbackHistory
		var durationNS int64

		err := rows.Scan(
			&entry.IncidentID, &entry.Service, &entry.Namespace, &entry.HelmRelease, &entry.OldRevision, &entry.RollbackRevision,
			&entry.StartTime, &entry.FinishTime, &durationNS, &entry.VerificationResult, &entry.OperatorVersion,
		)
		if err != nil {
			return nil, err
		}

		entry.RecoveryDuration = time.Duration(durationNS)
		list = append(list, &entry)
	}
	return list, nil
}
