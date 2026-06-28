# SRE Observability & Platform Dashboards Guide

This document details the required Grafana panels, Prometheus charts, and terminal logs to visualize the platform's self-healing capabilities.

---

## 1. Grafana Dashboards

### 1.1 SRE Control Plane Dashboard
- **Location**: `http://localhost:3000/dashboards`
- **Recommended Panels**:
  1. **Availability Score**: Single Stat gauge representing the platform's availability (target: `>99.9%`).
  2. **Active Incident Timeline**: Timeline graph showing when incidents are triggered, acknowledged, and resolved.
  3. **Error Budget Consumption**: Liquid fill or gauge representing the remaining error budget per service (e.g. `98.5%`).
  4. **Active Burn Rates**: Line chart tracking the rate of error budget consumption.

### 1.2 Recovery & MTTR Analytics Dashboard
- **Panels**:
  1. **Mean Time to Recovery (MTTR)**: Gauge displaying average MTTR (automated: `~90s` vs manual: `~30m`).
  2. **Total Rollbacks**: Counter tracking the number of automated rollbacks executed by the operator.
  3. **Verification Success Rate**: Pie chart showing the ratio of successful vs failed health verifications.

---

## 2. Prometheus Alerting UI
- **Location**: `http://localhost:9090/alerts`
- **Visuals**:
  1. **HighErrorRate (Active)**: Visualizing the rule transition from `Green` (Inactive) -> `Yellow` (Pending) -> `Red` (Firing).
  2. **Rule Query Graph**: Plotting `rate(http_requests_total{status_code="500"}[2m])` vs the 5% SLO threshold line.

---

## 3. Operator Execution Logs
- **Terminal Command**: `kubectl logs -f deployment/rollback-operator -n default`
- **Sample Output**:
  ```text
  [2026-06-28T13:00:00Z] LEVEL=INFO MSG="processing alert" TraceID=abc-123 IncidentID=inc-987 AlertName=HighErrorRate Service=order-service status=firing
  [2026-06-28T13:00:02Z] LEVEL=INFO MSG="acquired lease, executing rollback plan" IncidentID=inc-987 service=order-service target_revision=N-1
  [2026-06-28T13:00:05Z] LEVEL=INFO MSG="helm rollback succeeded" IncidentID=inc-987 release=order-service revision=1
  [2026-06-28T13:00:15Z] LEVEL=INFO MSG="health check verified, system restored" IncidentID=inc-987 status=UP MTTR=15.2s
  ```

---

## 4. Argo Rollouts Progressive Delivery
- **Command**: `kubectl argo rollouts get rollout payment-service -n default`
- **Visuals**:
  1. **Canary Breakdown**: Visualizing the revision tree: showing active stable pods (100%) alongside blocked or aborted canary replicas.
  2. **Blocked Rollout State**: Indicating `Paused (Blocked by Deployment Guard)`.
