# P11 — Self-Healing Kubernetes Cluster
## Automated Rollback via Prometheus Alerting Rules
### IIIT Bangalore M.Tech Project

---

## What Was Already Done (Your Starting Point)
- Single Go app (`mini_monitoring`) exposing Prometheus metrics on port 8080
- Standalone `prometheus.yml` scraping it via Docker (`host.docker.internal`)
- No Kubernetes, no Helm chart, no Alertmanager, no operator

## What This Repository Now Contains

```
Summer_Project/
├── cmd/
│   ├── order-service/main.go       ← Microservice 1: order management
│   ├── inventory-service/main.go   ← Microservice 2: inventory lookup
│   ├── payment-service/main.go     ← Microservice 3: payment processing
│   └── operator/main.go            ← Rollback operator (the core of the project)
├── internal/
│   └── server/server.go            ← Shared HTTP + metrics logic (used by all 3 services)
├── helm/
│   ├── order-service/              ← Helm chart for order-service
│   ├── inventory-service/          ← Helm chart for inventory-service
│   ├── payment-service/            ← Helm chart for payment-service
│   └── rollback-operator/          ← Helm chart for the operator itself
├── k8s/
│   ├── prometheus-rules/
│   │   └── slo-alerting-rules.yaml ← SLO thresholds (error rate, latency, availability)
│   ├── alertmanager/
│   │   └── alertmanager-config.yaml← Routes critical alerts to operator webhook
│   └── operator-rbac.yaml          ← RBAC permissions for the operator
├── services/
│   ├── order-service/Dockerfile
│   ├── inventory-service/Dockerfile
│   └── payment-service/Dockerfile
├── operator/
│   └── Dockerfile                  ← Includes helm binary (needed for helm rollback)
└── scripts/
    ├── 01-setup-cluster.sh         ← Start minikube + install kube-prometheus-stack
    ├── 02-build-and-deploy.sh      ← Build images + deploy all services
    ├── 03-generate-traffic.sh      ← Continuous traffic generator
    └── 04-inject-failure-and-measure-mttr.sh  ← Research experiment runner
```

---

## Step-by-Step: How to Run Everything

### Prerequisites (install these first)
```bash
# 1. Docker Desktop
# Download from: https://www.docker.com/products/docker-desktop

# 2. minikube
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube

# 3. kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# 4. Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Verify
minikube version
kubectl version --client
helm version
```

### Step 1: Start the Cluster + Install Prometheus Stack
```bash
chmod +x scripts/*.sh
./scripts/01-setup-cluster.sh
```
This starts minikube and installs Prometheus + Alertmanager + Grafana via `kube-prometheus-stack`.

### Step 2: Update go.mod module path (ONE TIME ONLY)
```bash
# Your current go.mod uses module "mini_monitoring" — update it to match the repo
# Change line 1 of go.mod from:
#   module mini_monitoring
# To:
#   module github.com/rohithsandiri/Summer_Project
```

### Step 3: Build Images and Deploy Everything
```bash
./scripts/02-build-and-deploy.sh
```
Builds 4 Docker images, loads them into minikube, deploys via Helm, applies alerting rules.

### Step 4: Open Monitoring UIs (3 terminals)
```bash
# Terminal 1 — Prometheus
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-prometheus 9090:9090

# Terminal 2 — Grafana  
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80

# Terminal 3 — Alertmanager
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-alertmanager 9093:9093
```

### Step 5: Start Traffic Generator
```bash
# Terminal 4 (keep running)
./scripts/03-generate-traffic.sh
```

### Step 6: Run Failure Injection Experiments
```bash
# Terminal 5
./scripts/04-inject-failure-and-measure-mttr.sh
```

---

## How Failure Injection Works

Each service reads `ERROR_RATE` and `LATENCY_MS` from environment variables.
You inject failures via `helm upgrade`:

```bash
# Inject 50% error rate into order-service (simulate bad deploy)
helm upgrade order-service ./helm/order-service \
    --set errorRate=0.5

# After ~60s: Prometheus detects SLO breach
# Alert fires → Alertmanager → webhook → operator → helm rollback
# order-service restores to previous revision automatically

# Inject 3s latency into inventory-service
helm upgrade inventory-service ./helm/inventory-service \
    --set latencyMs=3000

# Inject 80% errors into payment-service
helm upgrade payment-service ./helm/payment-service \
    --set errorRate=0.8
```

---

## Key PromQL Queries (test in Prometheus UI at localhost:9090)

```promql
# Error rate for order-service (should be < 5% for SLO)
rate(http_requests_total{job="order-service", status_code="500"}[2m])
/ rate(http_requests_total{job="order-service"}[2m])

# P99 latency for inventory-service
histogram_quantile(0.99,
  rate(http_request_duration_seconds_bucket{job="inventory-service"}[5m])
)

# MTTR histogram — your research result!
operator_mttr_seconds_bucket

# Total rollbacks triggered
operator_rollbacks_total

# Active requests per service
http_active_requests
```

---

## SLOs Implemented

| Service | Error Rate SLO | Latency SLO (P99) | Availability SLO |
|---------|---------------|-------------------|-----------------|
| order-service | < 5% | < 500ms | ≥ 1 pod healthy |
| inventory-service | < 5% | < 500ms | ≥ 1 pod healthy |
| payment-service | < 1% (stricter!) | < 200ms (stricter!) | ≥ 1 pod healthy |

---

## MTTR Measurement Protocol

```
T0 = helm upgrade runs (failure injected)
T1 = Prometheus detects SLO breach (after `for: 1m` window)
T2 = Alertmanager fires webhook to operator
T3 = Operator calls helm rollback
T4 = All pods healthy (rollback complete)

MTTR = T4 - T1  (detection to recovery)
```

After running all scenarios, compare:
- **Manual MTTR** (you observe alert → manually run helm rollback): ~15-45 minutes
- **Automated MTTR** (operator does it): ~90-180 seconds
- **Improvement factor**: 10-20x

---

## Checking Operator Logs
```bash
# Watch operator logs in real time
kubectl logs -f deployment/rollback-operator -n default

# Expected output when an alert fires:
# ALERT FIRING: default/order-service | alert=OrderServiceHighErrorRate
# ROLLBACK START: release=order-service namespace=default
# helm rollback output: Rollback was a success! Happy Helming!
# ROLLBACK SUCCESS: default/order-service | MTTR=107.3s
```

---

## Troubleshooting

**ServiceMonitor not picking up services:**
```bash
kubectl get servicemonitor -n default
# Should show order-service, inventory-service, payment-service
# If missing: check that release: monitoring label is in the ServiceMonitor
```

**Alerts not firing:**
```bash
# Check Prometheus rules are loaded
kubectl get prometheusrule -n monitoring
# Check rule evaluation in Prometheus UI → Status → Rules
```

**Operator not receiving webhooks:**
```bash
# Check Alertmanager config
kubectl get alertmanagerconfig -n monitoring
# Check operator is reachable from monitoring namespace
kubectl exec -n monitoring deployment/alertmanager -c alertmanager -- \
    wget -qO- http://rollback-operator.default.svc.cluster.local:8080/health
```

**Helm rollback failing in operator:**
```bash
# The operator needs helm binary in its container (see operator/Dockerfile)
# And needs the rollback-operator-sa ServiceAccount with RBAC
kubectl get clusterrolebinding rollback-operator-binding
```
