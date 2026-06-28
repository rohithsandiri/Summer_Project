# Architectural Visualization Diagrams

This document contains Mermaid.js visualizations mapping out the architecture, lifecycles, and flows of the self-healing cloud-native operator platform.

## 1. Overall System Architecture
```mermaid
graph TD
    Client["API Clients / Load Tests"] --> Gateway["API Gateway"]
    Gateway --> OrderSvc["Order Service"]
    Gateway --> PaySvc["Payment Service"]
    Gateway --> InvSvc["Inventory Service"]
    
    OrderSvc & PaySvc & InvSvc --> DB["PostgreSQL Database"]
    
    OrderSvc & PaySvc & InvSvc & Gateway -.-> Prometheus["Prometheus Operator / ServiceMonitors"]
    Prometheus --> Alertmanager["Alertmanager"]
    Alertmanager --> WebhookReceiver["Operator Webhook Handler"]
    
    subgraph "Self-Healing Operator Control Plane"
        WebhookReceiver --> IncidentMgr["Incident Manager"]
        IncidentMgr --> DecisionEngine["Decision Engine"]
        DecisionEngine --> PolicyEngine["Policy Engine"]
        IncidentMgr --> TimelineEngine["Timeline Engine"]
        IncidentMgr --> ReliabilityEngine["Reliability & MTTR Engine"]
        
        DecisionEngine --> RecoveryPlanner["Recovery Planner"]
        RecoveryPlanner --> RecoveryExecutor["Recovery Executor"]
        RecoveryExecutor --> HelmSDK["Helm Go SDK Executor"]
        HelmSDK --> K8sCluster["Kubernetes Cluster (API Server)"]
        
        RecoveryExecutor --> Verification["Verification Engine"]
        Verification --> K8sCluster
    end
```

---

## 2. Operator Internal Components & Dependency Wiring
```mermaid
graph LR
    subgraph "SRE Context & Data Sources"
        SLO["SLO Engine"]
        Budget["Error Budget Manager"]
        BurnRate["Burn Rate Engine"]
        RootCause["Root Cause Analyzer"]
        DepGraph["Dependency Graph"]
    end
    
    subgraph "Decision & Planning Core"
        Decision["Intelligent Decision Engine"]
        Policy["Policy Engine"]
    end
    
    SLO & Budget & BurnRate & RootCause --> Decision
    DepGraph --> RootCause
    
    Decision --> Policy
    Policy --> Planner["Recovery Planner"]
    Planner --> Executor["Recovery Executor"]
    
    subgraph "Observability & Timeline Logging"
        Timeline["SRE Timeline Engine"]
        RelEngine["Reliability Engine"]
    end
    
    Executor --> Timeline
    Executor --> RelEngine
```

---

## 3. Progressive Delivery Canary Deployment Flow (Argo Rollouts)
```mermaid
sequenceDiagram
    autonumber
    actor Developer
    participant K8s as Kubernetes API
    participant Argo as Argo Rollouts Controller
    participant Guard as Deployment Guard Engine
    participant Prom as Prometheus
    
    Developer->>K8s: Deploy bad release (Helm Upgrade)
    K8s->>Argo: Trigger Rollout
    Argo->>Argo: Scale up Canary Pods
    Argo->>Guard: Request Deployment Guard check
    Guard->>Prom: Query SLO burn rate & Error Budgets
    Prom-->>Guard: SLO violation / high burn rate detected
    Guard-->>Argo: Block rollout & trigger Rollout Abort
    Argo->>K8s: Initiate automated Canary rollback
```

---

## 4. Automated Recovery Flow (Self-Healing Loop)
```mermaid
sequenceDiagram
    autonumber
    participant AP as Alertmanager / Prometheus
    participant OM as Incident Manager (Operator)
    participant DE as Decision Engine
    participant RE as Recovery Executor (Helm SDK)
    participant VE as Verification Engine
    participant TE as Timeline Engine
    
    AP->>OM: POST alert JSON to /webhook
    OM->>TE: Record event "Alert Fired"
    OM->>DE: Query optimal mitigation action
    DE-->>OM: Action: Helm Rollback
    OM->>TE: Record event "Rollback Planned"
    OM->>RE: Trigger Rollback for release
    RE->>RE: Locate previous healthy revision
    RE-->>OM: Rollback complete
    OM->>TE: Record event "Rollback Completed"
    OM->>VE: Initiate Verification loops
    VE-->>OM: Verification successful
    OM->>TE: Record event "Incident Resolved"
```

---

## 5. Incident Lifecycle & FSM Transitions
```mermaid
stateDiagram-v2
    [*] --> Healthy
    Healthy --> Warning : Alert Fired (StartsAt)
    Warning --> Investigating : Operator analysis started
    Investigating --> DecisionMade : Optimal mitigation determined
    DecisionMade --> RecoveryPlanned : Recovery steps structured
    RecoveryPlanned --> ExecutingRollback : Helm SDK executing rollback
    ExecutingRollback --> RollbackComplete : Rollback command finished
    RollbackComplete --> Verifying : Verification checks running
    Verifying --> Recovered : System verified healthy
    Recovered --> Healthy : Alert resolved & archived
    
    Verifying --> Retrying : Verification failed / retryable
    Retrying --> ExecutingRollback : Execute next attempt
    
    Verifying --> Escalated : Max retries exceeded
    Escalated --> [*]
```

---

## 6. SRE Timeline & Analytics Logging Engine
```mermaid
graph TD
    Event["Operational Event (FSM change, Rollback, Verifier)"] --> Engine["Timeline Engine Interface"]
    
    subgraph "Timeline Engine Implementations"
        Engine --> SQL["SQL Timeline Engine"]
        Engine --> Mem["InMemory Timeline Engine"]
    end
    
    SQL --> DB["Postgres DB (timeline_events table)"]
    Mem --> Cache["InMemory Go Map & Mutex"]
    
    DB & Cache --> API["REST API /timelines?incident_id=xyz"]
    API --> UI["Grafana / Thesis Report / Runbook Generator"]
```
