# M.Tech Thesis & SRE Portfolio Presentation Outline

This document provides a slide-by-slide structure, core messages, and talking points for presenting the project to thesis committees or SRE interviewers.

---

### Slide 1: Title Slide
- **Title**: Evolving Microservice Reliability: A Self-Healing Control Plane Operator with Programmatic Helm Recovery
- **Subtitle**: M.Tech Thesis & SRE Portfolio Presentation
- **Presenter**: Rohith Sandiri

---

### Slide 2: Background & Motivation
- **Context**: Growth of Kubernetes and progressive delivery in cloud-native platforms.
- **The Problem**: Manual operations (SREs reacting to pages) are slow, error-prone, and lead to high MTTR.
- **Core Question**: Can we design an autonomous control loop that detects, validates, and rolls back regressions programmatically?

---

### Slide 3: System Architecture
- **API Gateway**: Entrypoint with distributed tracing, rate limiting, and circuit breakers.
- **Microservices**: Order, Payment, and Inventory services communicating via HTTP/Saga.
- **Observability**: Prometheus scraping `/metrics` and feeding Alertmanager.
- **Control Plane**: Rollback Operator subscribing to Alertmanager webhook.

---

### Slide 4: SRE Decision Engines
- **SLO Engine**: Evaluating availability and p95 latency thresholds.
- **Error Budget Manager**: Tracks remaining budget over sliding windows.
- **Burn Rate Engine**: Monitors speed of budget consumption.
- **RCA Engine**: Traverses service dependency graphs to identify the culprit.

---

### Slide 5: The Execution Plane (Helm SDK)
- **Why Helm SDK?**: Explain the integration of programmatic Go SDKs rather than executing sub-shell commands.
- **State Machine**: Highlight the lifecycle from Alert -> Triggered -> Evaluating -> Planning -> Executing -> Verifying -> Recovered.
- **Verification**: Real-time validation of pod status and health endpoints before completing recovery.

---

### Slide 6: Progressive Delivery & Deployment Guard
- **Argo Rollouts**: Integration for canary deployments.
- **Deployment Guard**: SRE gatekeeper checks:
  1. Active incidents.
  2. Depleted error budget.
  3. High burn rate.
- **Result**: Blocks risky deploys to protect the system.

---

### Slide 7: Evaluation & Benchmarks
- **Key Findings**:
  - Manual MTTR: `~14 minutes`.
  - Automated MTTR: `~76 seconds` (11x speedup).
  - Webhook processing latency: `<5ms`.
  - Rollback SDK invocation time: `<2s`.

---

### Slide 8: Conclusion & Future Work
- **Conclusion**: Built a production-grade, highly available, self-healing Kubernetes operator.
- **Future Directions**:
  - Machine learning based anomaly detection.
  - Multi-cluster lease locking and routing.
  - Automated canary analysis (ACA) using Prometheus metrics.
