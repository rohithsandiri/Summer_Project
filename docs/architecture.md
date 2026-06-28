# Platform & Operator Architecture Specification

This document details the high-level system architecture, microservice communication model, operator design, SRE decision/recovery engines, and runtime lifecycles.

---

## 1. System Architecture Overview

The Self-Healing Platform consists of an API Gateway, core business microservices, a custom Kubernetes Operator, a dedicated PostgreSQL store for incident tracking, and a Prometheus-based observability stack.

```mermaid
graph TD
    User([End User]) -->|HTTP Traffic| GW[API Gateway]
    
    subgraph "Application Layer (cloud-native-platform)"
        GW -->|Route /api/orders| OrderSvc[Order Service]
        GW -->|Route /api/inventory| InvSvc[Inventory Service]
        GW -->|Route /api/payments| PaySvc[Payment Service]
        OrderSvc -->|Internal HTTP| InvSvc
        OrderSvc -->|Internal HTTP| PaySvc
    end

    subgraph "Observability Stack (monitoring)"
        Prom[Prometheus Server] -.->|Scrapes /metrics| OrderSvc
        Prom -.->|Scrapes /metrics| InvSvc
        Prom -.->|Scrapes /metrics| PaySvc
        Prom -.->|Scrapes /metrics| GW
        Prom -->|Triggers Alerts| AM[Alertmanager]
    end

    subgraph "Control & Execution Plane"
        AM -->|Webhook Alert| Op[Rollback Operator]
        Op -->|Helm SDK Rollback| K8sAPI[K8s API Server]
        Op -->|Read/Write Incidents| DB[(PostgreSQL)]
        K8sAPI -.->|Updates Workloads| OrderSvc
    end
```

---

## 2. Microservice Communication & Spans

Distributed tracing uses OpenTelemetry (OTel) with W3C propagation. The API Gateway generates `TraceID`s (if not present) and propagates them downstream.

```mermaid
sequenceDiagram
    autonumber
    actor User as User Client
    participant GW as API Gateway
    participant Order as Order Service
    participant Inv as Inventory Service
    participant Pay as Payment Service

    User->>GW: POST /api/orders (No TraceID)
    Note over GW: Generates TraceID & span
    GW->>Order: POST /orders (TraceID propagated)
    Note over Order: Starts child span
    Order->>Inv: POST /inventory/reserve (TraceID propagated)
    Note over Inv: Reserves items
    Inv-->>Order: 200 OK
    Order->>Pay: POST /payment/process (TraceID propagated)
    Note over Pay: Charges wallet
    Pay-->>Order: 200 OK
    Order-->>GW: 201 Created
    GW-->>User: 201 Created (TraceID in header)
```

---

## 3. Operator Internal Architecture

The Rollback Operator acts as a custom controller. It consists of multiple independent engines executing in an asynchronous pipeline:

```mermaid
graph LD
    AM[Alertmanager] -->|Webhook| WebhookHandler[Webhook Handler]
    WebhookHandler -->|Parse Alert| IncidentMgr[Incident Manager]
    
    subgraph "SRE Control Plane"
        IncidentMgr -->|Query SLOs| SLO[SLO Manager]
        IncidentMgr -->|Check Burn Rate| BurnRate[Burn Rate Engine]
        IncidentMgr -->|Check Budget| Budget[Error Budget Manager]
        IncidentMgr -->|Analyze Dependencies| DepGraph[Dependency Graph]
        IncidentMgr -->|Find Root Cause| RCA[Root Cause Analyzer]
    end

    subgraph "Policy & Decision Engines"
        IncidentMgr -->|Evaluate Policies| Policy[Policy Engine]
        IncidentMgr -->|Compute Confidence| Decision[Decision Engine]
    end

    subgraph "SRE Execution Plane"
        IncidentMgr -->|Create Recovery Plan| Planner[Recovery Planner]
        IncidentMgr -->|Execute Plan| Executor[Recovery Executor]
        Executor -->|Call Helm SDK| HelmSDK[Helm Go SDK]
        Executor -->|Verify Health| Verifier[Verification Engine]
        Executor -->|Handle Failures| Retry[Retry Engine]
    end

    IncidentMgr -->|Audit History| DB[(PostgreSQL)]
    IncidentMgr -->|Timeline Audit| Timeline[Timeline Engine]
    IncidentMgr -->|Generate Recommendations| RecEngine[Recommendation Engine]
```

---

## 4. Incident Lifecycle State Machine

Each firing alert generates an incident that progresses through a strict state machine:

```mermaid
stateDiagram-v2
    [*] --> Healthy : Start
    Healthy --> Triggered : Alert Fires
    Triggered --> Evaluating : Policy/Decision Matches
    Evaluating --> Cooldown : Active Cooldown Exists
    Cooldown --> Evaluating : Cooldown Expired
    Evaluating --> Planning : Action Approved
    Planning --> Executing : Plan Generated
    Executing --> Verifying : Rollback Done
    Verifying --> Recovered : Health Verification Successful
    Verifying --> Retrying : Verification Failed (Attempts < Max)
    Retrying --> Executing : Retry Interval Expired
    Verifying --> Failed : Verification Failed (Attempts >= Max)
    Recovered --> Healthy : Alert Resolved & Cleared
    Failed --> ManualIntervention : Human Action Needed
```

---

## 5. Deployment Guard & Progressive Delivery Flow

When a new canary release is initiated (e.g. via Argo Rollouts), the operator evaluates system risk and blocks dangerous deployments:

```mermaid
sequenceDiagram
    autonumber
    actor Engineer as SRE Engineer
    participant Argo as Argo Rollouts
    participant Guard as Deployment Guard
    participant Risk as Risk Engine
    participant DB as Postgres Store

    Engineer->>Argo: Initiate Canary Rollout
    Argo->>Guard: Evaluate Deployment Safety
    Guard->>DB: Query Active Incidents & SLO status
    Guard->>Risk: Calculate Deployment Risk Score
    Note over Risk: Evaluates Budget Burn & Service Deps
    Risk-->>Guard: Risk Score (e.g., 0.95 - High Risk)
    alt High Risk or Active Incident
        Guard-->>Argo: Block Rollout (Safety Guard Tripped)
        Argo-->>Engineer: Rollout Aborted
    else Low Risk & No Incidents
        Guard-->>Argo: Approve Rollout
        Argo->>Engineer: Progress Canary (10% -> 50% -> 100%)
    end
```
