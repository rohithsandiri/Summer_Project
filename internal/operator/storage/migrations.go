// internal/operator/storage/migrations.go
//
// Versioned, forward-only migrations for the operator's own Postgres
// incident-audit store, plus a startup compatibility guard.
//
// Context: the platform's three business microservices (order, inventory,
// payment) hold no persistent data at all — their repositories are in-memory
// (see services/*/repository/repository.go), so a Helm rollback of those
// services cannot lose or corrupt application data; a pod restart already
// resets their state regardless of which revision is live. The ONE real,
// persistent schema anywhere in this platform is this store, used by the
// operator itself to keep incidents, decisions, and rollback history.
//
// That schema evolves like any other: columns get added, indexes get added,
// and eventually a column or table needs to be dropped or renamed. The old
// migrate() just ran a fixed list of `CREATE TABLE IF NOT EXISTS` statements
// on every boot — safe for purely additive changes, but with no version
// tracking there was no way to reason about what happens when the operator's
// OWN Helm release (helm/rollback-operator) is rolled back to a build whose
// code predates a schema change that already ran against the live database.
//
// Expand/contract discipline for future migrations:
//  1. Never drop or rename a column/table in the same migration that stops
//     using it. First stop reading/writing it (an "expand" migration, safe
//     for old code to coexist with), deploy, observe, THEN remove it in a
//     later migration.
//  2. Any migration that drops/renames a column or table, changes a column's
//     type incompatibly, or adds a NOT NULL column with no default MUST set
//     Breaking: true. That flag is what lets an older binary detect it can no
//     longer safely run against the current schema (see checkCompatibility).
//  3. Additive changes (new nullable column, new index, new table) are not
//     breaking: old code that doesn't know about them keeps working exactly
//     as before, since every query here uses explicit column lists rather
//     than `SELECT *`.
package storage

import (
	"fmt"
	"time"
)

// migration is one forward-only schema change, applied at most once and
// recorded permanently in schema_migrations.
type migration struct {
	Version     int
	Description string
	// Breaking marks a migration that an older binary cannot safely operate
	// against (see the discipline above). Non-breaking migrations are always
	// safe to leave applied even if the running binary predates them.
	Breaking bool
	// Statements run in order, in a single transaction, per migration.
	Statements []string
}

// migrations is the complete, ordered history of schema changes this build
// knows about. Only ever append; never edit or remove a past entry.
var migrations = []migration{
	{
		Version:     1,
		Description: "baseline: incidents, timeline_events, rollback_history, decision_history, knowledge_base",
		Breaking:    false,
		Statements: []string{
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
		},
	},
	{
		Version:     2,
		Description: "add lookup indexes for dashboard/runbook queries (service, incident_id)",
		Breaking:    false,
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_incidents_service ON incidents(service);`,
			`CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(status);`,
			`CREATE INDEX IF NOT EXISTS idx_rollback_history_service ON rollback_history(service);`,
			`CREATE INDEX IF NOT EXISTS idx_rollback_history_incident_id ON rollback_history(incident_id);`,
			`CREATE INDEX IF NOT EXISTS idx_decision_history_incident_id ON decision_history(incident_id);`,
		},
	},
}

// RequiredSchemaVersion is the highest migration this build's query code
// depends on. It is derived from migrations itself rather than hand-bumped,
// so it can never drift from what the list above actually contains.
var RequiredSchemaVersion = highestVersion(migrations)

func highestVersion(ms []migration) int {
	max := 0
	for _, m := range ms {
		if m.Version > max {
			max = m.Version
		}
	}
	return max
}

// appliedMigration is one row read back from schema_migrations.
type appliedMigration struct {
	Version     int
	Description string
	Breaking    bool
}

// checkCompatibility is the startup safety net. It is a pure function (no DB
// access) so it can be unit tested directly: given every migration recorded
// as applied against this database and the version this binary's code was
// built against, decide whether it is safe to proceed.
//
// The scenario this guards against: the operator's own Helm release
// (helm/rollback-operator) gets rolled back — by a human, by CI, or in
// principle by the operator's own automated pipeline — to a build older than
// a breaking migration that already ran against the live database. That old
// binary's queries (explicit column lists, not SELECT *) would either fail
// outright on a renamed/dropped column, or — worse — silently misinterpret
// a repurposed one. Refusing to start with a clear error is strictly safer
// than either of those outcomes.
func checkCompatibility(applied []appliedMigration, requiredVersion int) error {
	for _, m := range applied {
		if m.Breaking && m.Version > requiredVersion {
			return fmt.Errorf(
				"refusing to start: database schema is ahead by breaking migration %d (%q), "+
					"but this operator build only understands schema version %d. "+
					"This looks like the rollback-operator Helm release was rolled back to a build "+
					"that predates a breaking data migration. Roll forward to a build with schema "+
					"version >= %d instead, or restore the database from a backup taken before that "+
					"migration ran",
				m.Version, m.Description, requiredVersion, m.Version,
			)
		}
	}
	return nil
}

// migrate creates the schema_migrations ledger if absent, applies every
// migration this binary knows about that isn't recorded yet (each in its own
// transaction, so a failure partway through a migration can't leave the
// schema half-changed), then runs the compatibility guard against the full
// set of applied migrations — including any recorded by a newer binary that
// has since been rolled back.
func (s *PostgresIncidentStore) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INT PRIMARY KEY,
		description TEXT NOT NULL,
		breaking BOOLEAN NOT NULL DEFAULT false,
		applied_at TIMESTAMP WITH TIME ZONE NOT NULL
	);`); err != nil {
		return fmt.Errorf("failed to create schema_migrations ledger: %w", err)
	}

	applied, err := s.appliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to read schema_migrations: %w", err)
	}
	alreadyApplied := make(map[int]bool, len(applied))
	for _, a := range applied {
		alreadyApplied[a.Version] = true
	}

	for _, m := range migrations {
		if alreadyApplied[m.Version] {
			continue
		}
		if err := s.applyMigration(m); err != nil {
			return err
		}
	}

	applied, err = s.appliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to re-read schema_migrations after apply: %w", err)
	}
	return checkCompatibility(applied, RequiredSchemaVersion)
}

func (s *PostgresIncidentStore) applyMigration(m migration) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("migration %d (%s): begin transaction: %w", m.Version, m.Description, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if Commit already ran

	for _, stmt := range m.Statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration %d (%s) failed, rolled back: %w", m.Version, m.Description, err)
		}
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (version, description, breaking, applied_at) VALUES ($1, $2, $3, $4)`,
		m.Version, m.Description, m.Breaking, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("migration %d (%s): recording ledger entry: %w", m.Version, m.Description, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d (%s): commit: %w", m.Version, m.Description, err)
	}
	return nil
}

func (s *PostgresIncidentStore) appliedMigrations() ([]appliedMigration, error) {
	rows, err := s.db.Query(`SELECT version, description, breaking FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []appliedMigration
	for rows.Next() {
		var a appliedMigration
		if err := rows.Scan(&a.Version, &a.Description, &a.Breaking); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
