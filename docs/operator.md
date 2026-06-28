# Self-Healing Operator Architecture & Component Guide

This document provides a detailed breakdown of the custom Kubernetes Operator's internal modules, SRE engines, and failover mechanics.

---

## 1. Webhook Receiver & Parser
- **Package**: `internal/operator/webhook` and `internal/operator/alert`
- **Function**: Receives HTTP `POST` payloads from Alertmanager, parses raw alerts, aggregates them by alert fingerprint, and triggers the `IncidentManager`.

---

## 2. SRE Control Plane Engines

### 2.1 SLO Manager (`internal/operator/slo`)
Evaluates target availability, latency targets (p95/p99), and error rate ceilings. It queries metrics directly from Prometheus to assess the depth of the breach.

### 2.2 Burn Rate Engine & Error Budget Manager (`internal/operator/burnrate` & `internal/operator/budget`)
Calculates the rate at which a service is consuming its remaining error budget (burn rate: 14.4x consumes 2% of budget in 1 hour; 1.0x consumes budget over 30 days). If the remaining budget drops below `10%` or the burn rate exceeds `14.4`, the recovery priority is elevated.

### 2.3 Dependency Graph & Root Cause Analyzer (`internal/operator/dependency` & `internal/operator/rootcause`)
- **Dependency Graph**: Builds a map of microservice dependencies (e.g. `api-gateway` -> `order-service` -> `payment-service`).
- **RCA Analyzer**: Evaluates which service in the graph is the root cause by cross-referencing latency peaks and error metrics.

---

## 3. Decision & Policy Engines
- **Policy Engine (`internal/operator/policy`)**: Checks configured cooldown rules (e.g. "Do not rollback a service if a rollback was executed in the last 5 minutes").
- **Decision Engine (`internal/operator/decision`)**: Compiles metrics from SLO, Budget, Burn Rate, and RCA, outputting a recommended action (`ActionPrepareRollback`, `ActionWait`, etc.) and a confidence score between `0.0` and `1.0`.

---

## 4. Recovery Planner & Executor
- **Recovery Planner (`internal/operator/planner`)**: Generates structured, step-by-step recovery plans (e.g. "Step 1: Execute Helm rollback to version N-1; Step 2: Validate health endpoints; Step 3: Trigger post-rollback verification").
- **Recovery Executor (`internal/operator/executor`)**: Implements the Helm Go SDK calls to execute the rollback.

---

## 5. Verification & Retry Engines
- **Verification Engine (`internal/operator/verification`)**: Queries the Kubernetes API server using `client-go` to check deployment status, replica availability, and pod health endpoints until a stable state is achieved.
- **Retry Engine (`internal/operator/retry`)**: Implements exponential backoff retry cycles if health verification fails.

---

## 6. High-Availability (Leader Election)
- Implemented using client-go's `leaderelection` framework.
- Restricts processing of alert webhooks and progressive rollouts to the active leader lease holder.
- Promotes a standby replica automatically within 10-15 seconds in case of leader failure.
