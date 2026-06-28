# Self-Healing Kubernetes Platform: Automated SRE Control Plane

[![Go Version](https://img.shields.io/github/go-mod/go-version/rohithsandiri/Summer_Project)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/kubernetes-v1.28%2B-blue)](https://kubernetes.io)
[![Helm](https://img.shields.io/badge/helm-v3-orange)](https://helm.sh)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

An enterprise-grade, cloud-native self-healing platform for automated recovery and progressive delivery validation. This project implements an active **SRE Control and Execution Plane** that transforms Prometheus alerts into structured recovery plans and executes programmatic Helm SDK rollbacks.

---

## 📖 Table of Contents
1. [System Architecture](#-system-architecture)
2. [SRE Engines & Workflow](#-sre-engines--workflow)
3. [Technology Stack](#-technology-stack)
4. [Quick Start Guide](#-quick-start-guide)
5. [Local Development](#-local-development)
6. [Multi-Environment Configuration](#-multi-environment-configuration)
7. [Observability & Dashboarding](#-observability--dashboarding)
8. [Automated Rollback & Verification](#-automated-rollback--verification)
9. [progressive-delivery](#-progressive-delivery--deployment-guard)
10. [Academic Thesis & Assets](#-academic-thesis--assets)
11. [License](#-license)

---

## 🏗 System Architecture

The platform consists of an API Gateway acting as a single entrypoint, three core business microservices, a custom Kubernetes Operator, a dedicated PostgreSQL store for incident audit trails, and a Prometheus-based observability stack.

```
                  +-----------------------------------+
                  |            End User               |
                  +-----------------------------------+
                                    |
                                    | HTTP Traffic
                                    v
                  +-----------------------------------+
                  |            API Gateway            |
                  +-----------------------------------+
                   /                |                \
    /api/orders   /  /api/inventory |  /api/payments  \
                 v                  v                  v
        +-----------+         +-----------+         +-----------+
        |   Order   |-------->| Inventory |         |  Payment  |
        |  Service  |         |  Service  |         |  Service  |
        +-----------+         +-----------+         +-----------+
              \                     |                     /
               \                    |                    /
                +-------------------+-------------------+
                                    |
                                    v (Scrapes /metrics)
                  +-----------------------------------+
                  |         Prometheus Server         |
                  +-----------------------------------+
                                    |
                                    v (Triggers Alert)
                  +-----------------------------------+
                  |           Alertmanager            |
                  +-----------------------------------+
                                    |
                                    v (Webhook Payload)
                  +-----------------------------------+
                  |      Rollback Operator (Leader)   |
                  +-----------------------------------+
                    /               |               \
   Helm Go SDK     /    Audit Trails|    Verify Health\
                  v                 v                  v
            +-----------+     +-----------+      +-----------+
            |  K8s API  |     | PostgreSQL|      |  Target   |
            |  Server   |     |    DB     |      | Workloads |
            +-----------+     +-----------+      +-----------+
```

For a detailed breakdown of communication channels, lifecycles, and sequence diagrams, refer to the [Architecture Documentation](docs/architecture.md).

---

## ⚙ SRE Engines & Workflow

The Rollback Operator operates as an asynchronous, state-driven control loop:
1. **SLO Evaluator**: Checks real-time availability and response latencies against targets.
2. **Burn Rate & Error Budget Manager**: Measures the speed at which errors consume the budget.
3. **Dependency Graph & Root Cause Analyzer (RCA)**: Pinpoints the culprit service in a cascading outage.
4. **Policy & Decision Engine**: Evaluates safety cooldown rules and issues recovery plans with a confidence rating.
5. **Helm Rollback Executor**: Connects to the **Helm Go SDK** to perform version rollbacks programmatically.
6. **Health Verifier**: Continuously checks pod readiness and service status before finishing the recovery.

---

## 🛠 Technology Stack

- **Backend**: Go (Golang) 1.21
- **API Gateway**: Custom Reverse Proxy with Rate Limiting, Circuit Breakers, and OTel propagation
- **Database**: PostgreSQL (Persistent Store) & Redis (Cache)
- **Containerization**: Docker & Docker Compose
- **Orchestration**: Kubernetes (Deployments, Services, ConfigMaps, Secrets, RBAC, NetworkPolicies, PDBs)
- **Packaging**: Helm 3
- **Observability**: Prometheus Operator, ServiceMonitors, PrometheusRules, Alertmanager, Grafana Dashboards
- **Distributed Tracing**: OpenTelemetry & Jaeger
- **Progressive Delivery**: Argo Rollouts (Canary deployments)

---

## 🚀 Quick Start Guide

### Prerequisites
Make sure you have installed:
- Docker Desktop
- minikube
- kubectl
- Helm 3

### Deployment in 3 Steps
1. **Initialize the Cluster & Monitoring Stack**:
   ```bash
   ./scripts/01-setup-cluster.sh
   ```
2. **Build and Deploy Platform Workloads**:
   ```bash
   ./scripts/02-build-and-deploy.sh
   ```
3. **Start Load Generator**:
   ```bash
   ./scripts/03-generate-traffic.sh
   ```

---

## 💻 Local Development

### Running with Docker Compose
If you want to run and test the microservices locally without Kubernetes:
```bash
docker-compose up --build
```
This starts the Postgres database, API Gateway, Order Service, Inventory Service, and Payment Service.

---

## 🌐 Multi-Environment Configuration

Each Helm chart includes three separate values profiles:
- **Dev**: Single replica, minimum resource limits, debug logging. See [values-dev.yaml](helm/order-service/values-dev.yaml).
- **Staging**: Dual replicas, standard limits, enabled Prometheus ServiceMonitors. See [values-staging.yaml](helm/order-service/values-staging.yaml).
- **Production**: Three replicas, high limits, pod anti-affinities, TopologySpreadConstraints, and securityContext locks. See [values-prod.yaml](helm/order-service/values-prod.yaml).

---

## 📊 Observability & Dashboarding

Prometheus Operator is configured via `ServiceMonitor` resources to automatically scrape endpoints every 15s. Custom **Grafana Dashboards** are loaded at deployment:
- **SRE Reliability Dashboard**: Displays SLO status, burn rates, remaining error budgets, and MTTR graphs.
- **Microservices Performance**: Shows requests per second, error ratios, and p95/p99 response latency.

Refer to the [Screenshots Guide](docs/screenshots.md) to view layout setups.

---

## 🛡 Automated Rollback & Verification

To inject a failure and verify the self-healing capability:
```bash
# Inject 50% error rate on order-service
helm upgrade order-service ./helm/order-service --set errorRate=0.5
```
Watch the operator automatically rollback the release to the previous healthy version `N-1`. Refer to the [Demo Guide](docs/demo-guide.md) for full interactive scripts.

---

## 🚦 Progressive Delivery & Deployment Guard

When executing a canary rollout:
- The **Deployment Guard** evaluates safety conditions (active incidents, SLO breaches, error budget depletion).
- If the deployment risk score calculated by the **Risk Engine** is too high, the rollout is blocked.
- Argo Rollouts pauses and aborts the canary release to protect production traffic.

---

## 🎓 Academic Thesis & Assets

This project is prepared for M.Tech thesis submission at IIIT Bangalore. All research, abstract, evaluation metrics, and performance charts are located under:
- **Thesis Abstract & Problem Statement**: [Thesis Assets](docs/thesis-assets.md)
- **Performance Benchmarks (Manual vs Automated)**: [Benchmark Report](docs/benchmarks.md)
- **Defense Slide Deck Outline**: [Presentation Outline](docs/presentation.md)

---

## 📄 License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.
