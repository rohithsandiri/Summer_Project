# Enterprise SRE Reliability Platform Documentation

This document describes the Architecture, Database Schema, REST APIs, Recommendation Engine, and Runbook Formats of the Enterprise SRE Reliability Platform.

## 1. Database Schema & Persistence Layer

The storage layer persists incident states, timeline events, rollback history, policy decision history, and knowledge base patterns. It is backed by PostgreSQL, with an automatic in-memory fallback for local development and test environments.

### Table Schema

#### `incidents`
Stores persistent status of alarms, SLO validations, and recovery pipeline runs:
- `id` (VARCHAR primary key)
- `alert_id` (VARCHAR)
- `service` (VARCHAR)
- `namespace` (VARCHAR)
- `severity` (VARCHAR)
- `status` (VARCHAR)
- `current_state` (VARCHAR)
- `start_time` (TIMESTAMP)
- `end_time` (TIMESTAMP)
- `duration_seconds` (DOUBLE PRECISION)
- `recovery_attempts` (INTEGER)
- `root_cause` (TEXT)
- `decision` (VARCHAR)
- `recovery_action` (VARCHAR)
- `helm_release` (VARCHAR)
- `rollback_revision` (INTEGER)
- `verification_result` (VARCHAR)
- `operator_version` (VARCHAR)
- `recovery_confidence` (DOUBLE PRECISION)

#### `timeline_events`
Stores chronologically ordered actions taken by the self-healing state machine:
- `id` (VARCHAR primary key)
- `incident_id` (VARCHAR references incidents)
- `timestamp` (TIMESTAMP)
- `message` (TEXT)

#### `rollback_history`
Stores historical metadata about execution outcomes of Helm Rollback commands:
- `id` (SERIAL primary key)
- `incident_id` (VARCHAR)
- `service` (VARCHAR)
- `namespace` (VARCHAR)
- `helm_release` (VARCHAR)
- `old_revision` (INTEGER)
- `rollback_revision` (INTEGER)
- `start_time` (TIMESTAMP)
- `finish_time` (TIMESTAMP)
- `recovery_duration_ms` (BIGINT)
- `verification_result` (VARCHAR)
- `operator_version` (VARCHAR)

#### `decision_history`
Stores recommendations and match reasons from the Decision Engine:
- `id` (SERIAL primary key)
- `incident_id` (VARCHAR)
- `policy_id` (VARCHAR)
- `decision` (VARCHAR)
- `reason` (TEXT)
- `confidence` (DOUBLE PRECISION)
- `timestamp` (TIMESTAMP)

#### `knowledge_base`
Stores historical patterns of successful recoveries used by the recommendation engine:
- `service` (VARCHAR)
- `failure_pattern` (VARCHAR)
- `action_taken` (VARCHAR)
- `outcome` (VARCHAR)

---

## 2. REST API Endpoints

The Operator HTTP Server exposes public REST APIs under the following routes:

### `GET /incidents`
Retrieves a list of all incidents or a specific incident details if `id` is specified.
- **Parameters**: `id` (optional)
- **Response**: JSON array of incident objects or single incident.

### `GET /incidents/active`
Retrieves a list of currently firing / active critical and warning incidents.
- **Response**: JSON array.

### `GET /incidents/history`
Retrieves all historically resolved incidents.
- **Response**: JSON array.

### `GET /analytics`
Calculates and returns Mean Time to Detect (MTTD), Mean Time to Recover (MTTR), Mean Time Between Failures (MTBF), average verification, and recovery times.
- **Parameters**: `service` (optional filter)
- **Response**:
```json
{
  "mttr": {
    "scope": "service",
    "target": "payment-service",
    "mttd_seconds": 15,
    "mttr_seconds": 235,
    "mtbf_seconds": 86400,
    "avg_rollback_time_seconds": 45,
    "avg_verification_time_seconds": 30
  },
  "reliability": {
    "availability": 0.9997,
    "incident_frequency": 3,
    "recovery_success_rate": 100.0,
    "reliability_score": 98.4
  }
}
```

### `GET /runbooks`
Generates post-mortem runbooks for an incident.
- **Parameters**: `incident_id` (required), `format` (optional: `markdown` / `html` / `json`)
- **Response**: Formatted text or HTML body.

### `GET /reliability`
Retrieves 0-100 SRE reliability scores and availability summaries.
- **Parameters**: `service` (optional)

### `GET /recommendations`
Returns recommended mitigation paths with dynamic confidence levels.
- **Parameters**: `service` (required)

### `GET /timelines`
Retrieves a chronological list of actions taken on an incident.
- **Parameters**: `incident_id` (required)

---

## 3. Recommendation Engine & Recovery Confidence

The recommendation engine calculates recovery suggestions based on SLO budgets, burn rates, and historical recoveries:
1. **Rollback**: Recommended with highest confidence (~95%) when error budgets are low and burn rates are extremely high.
2. **Scale**: Recommended for high traffic latency patterns.
3. **Restart**: Default low-risk recovery action.
4. **Observe**: Recommended if burn rate is low and error budget remains intact.
5. **Escalate**: Triggered if recovery attempts are exhausted or rollback fails.

Confidence is dynamically reduced if recent failures are identified in the database, protecting the cluster from feedback loops.
