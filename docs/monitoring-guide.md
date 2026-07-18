# Monitoring Guide — Where to Watch the Self-Healing Platform

Every port-forward below assumes the cluster from `scripts/01` + `scripts/02` (workloads in `default`, kube-prometheus-stack in `monitoring`). Run each in its own terminal and leave it open.

---

## 1. Grafana — dashboards (start here)

```bash
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80
# → http://localhost:3000   (admin / admin123)
```

Import the JSON dashboards from `k8s/monitoring/dashboards/` (Dashboards → New → Import) if not already present:

| Dashboard | What it shows |
|---|---|
| `sre-control-plane.json` | The operator itself: alerts received, incidents, decisions, rollbacks, MTTR |
| `recovery.json` | Recovery timelines and rollback outcomes |
| `order.json` / `inventory.json` / `payment.json` | Per-service golden signals (rate, errors, duration) |
| `reliability-platform.json` | Error budgets and burn rates |
| `progressive-delivery.json` | Canary / Argo Rollouts analysis |

## 2. Prometheus — raw metrics and alert states

```bash
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-prometheus 9090:9090
# → http://localhost:9090
```

Useful queries while running failure simulations:

```promql
ALERTS{alertstate="firing"}                       # everything firing right now
operator_rollbacks_total                          # rollbacks executed per service
operator_recovery_duration_seconds_bucket         # MTTR distribution
sum(rate(http_requests_total{status_code="500"}[2m])) by (job)
  / sum(rate(http_requests_total[2m])) by (job)   # live error rate per service
```

Status → Rules shows the SLO rules (including the stuck-rollout group) and how far each is from firing.

## 3. Alertmanager — alert routing

```bash
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-alertmanager 9093:9093
# → http://localhost:9093
```

Shows firing alerts grouped by `helm_release`, and which receiver they routed to (`rollback-operator` for critical, `null` otherwise).

## 4. The operator's own API — incidents, runbooks, analytics

```bash
kubectl port-forward -n default svc/rollback-operator 8090:8090
```

| Endpoint | Content |
|---|---|
| `GET /incidents` | All incidents (JSON) |
| `GET /incidents/active` | Currently open incidents |
| `GET /incidents/history` | Closed incidents with durations |
| `GET /runbooks?incident_id=<id>&format=markdown` | **Auto-generated runbook**: root cause, timeline, recovery actions, MTTR, manual steps (formats: json / markdown / html) |
| `GET /timelines` | Event-by-event incident timelines |
| `GET /analytics` | Aggregate MTTR / success-rate stats |
| `GET /recommendations` | Knowledge-base suggestions from past recoveries |
| `GET /metrics` | Operator Prometheus metrics |

Typical flow after a simulation: `curl -s localhost:8090/incidents/history | python3 -m json.tool` to find the incident ID, then fetch its runbook in markdown.

## 5. Jaeger — distributed traces

```bash
kubectl apply -f k8s/monitoring/jaeger.yaml        # once
kubectl port-forward -n monitoring svc/jaeger 16686:16686
# → http://localhost:16686
```

The services export OTLP/HTTP spans (see `internal/shared/otel`); the charts point `OTEL_EXPORTER_OTLP_ENDPOINT` at `jaeger:4318` by default. Select a service (e.g. `order-service`) in the UI to follow a request across order → inventory/payment, including spans during chaos injection.

## 6. Logs

Operator (structured JSON, one line per pipeline step — the primary audit trail):

```bash
kubectl logs -f deploy/rollback-operator -n default
```

Services:

```bash
kubectl logs -f deploy/order-service -n default
```

**Centralized logging (honest note):** the cluster is provisioned at 6 GB; a Loki + promtail stack fits but leaves little headroom for the failure simulations themselves. If you want it:

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm install loki grafana/loki-stack -n monitoring \
  --set promtail.enabled=true --set grafana.enabled=false
```

then add Loki (`http://loki:3100`) as a Grafana data source. Until then, `kubectl logs` plus the operator's `/timelines` endpoint cover the same investigative ground for this cluster.

## 7. Automated failure simulations

```bash
./scripts/05-run-failure-suite.sh          # all five scenarios, ~30-40 min
./scripts/05-run-failure-suite.sh s1 s5    # a subset
```

Non-interactive by design: each run measures MTTR per scenario, verifies the rollback landed on a clean configuration, restores the cluster on any failure, and writes a timestamped report to `docs/experiments/suite-*.md`. Because it needs a running cluster with traffic, schedule it from cron on the host (example in the script header) rather than CI.
