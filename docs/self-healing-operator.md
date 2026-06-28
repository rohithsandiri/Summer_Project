# Self-Healing Platform Control Plane (Phase 4A Operator)

This document provides a comprehensive overview of the **Self-Healing Control Plane Operator**, acting as the "brain" of the platform. Its primary responsibility is to consume Alertmanager webhooks, track incidents, match them against policies, execute a state machine, and compile structured **Recovery Plans** to be consumed by the execution plane (Phase 4B).

---

## 1. Directory & Package Structure

The operator is organized with isolated responsibilities:

```
cmd/operator/
  main.go                     # Entrypoint, dependency injection, HTTP boot
internal/operator/
  alert/
    parser.go                 # Webhook JSON validation & model parsing
  config/
    config.go                 # Environment configurations & default policy registry
  logger/
    logger.go                 # Contextual structured logger injecting Trace ID, Incident, State
  interfaces/
    interfaces.go             # Storage and engine decoupled contract definitions
  models/
    models.go                 # Strongly-typed domain models (Alert, Incident, Plan, etc.)
  webhook/
    handler.go                # POST /webhook controller with Trace ID propagation
  incident/
    manager.go                # Pipeline orchestrator managing incident lifecycles
  policy/
    engine.go                 # Matchmaker mapping alert targets to recommended actions
  decision/
    engine.go                 # Brain resolving cooldowns, limits, policies to decisions
  planner/
    planner.go                # Compiler translating decisions into RecoveryPlans
  state/
    machine.go                # FSM enforcing validated incident state transitions
  storage/
    inmemory.go               # Thread-safe local storage (replaces with Postgres later)
  utils/
    cooldown.go               # Thread-safe cooldown window manager
  metrics/
    metrics.go                # Custom Prometheus metrics registration
```

### Why each package exists:
- **`models`**: Enforces Go strict-typing across the control plane. Prevents typing mistakes associated with `map[string]interface{}`.
- **`alert`**: Decouples network JSON parsing from business operations. Performs validation and rejects malformed requests immediately.
- **`webhook`**: Isolates network protocol details (HTTP handlers, status codes). Handles trace context creation.
- **`incident`**: Orchestrates the pipeline. Coordinates storage updates, policy matching, state FSM steps, and planner execution.
- **`policy`**: Contains policy configurations mapping alerts to actions. Kept modular to support external DB storage or ConfigMap loading.
- **`decision`**: Solves complex logical checks (limit constraints, cooldown periods, retries) and decides the action.
- **`planner`**: Compiles the desired action, verification windows, timeouts, and priorities into a schema ready for execution.
- **`state`**: Regulates transition validations. Ensures an incident cannot skip workflow steps (e.g. from Firing to Recovered without investigation).
- **`storage`**: Abstracts persistent state, decoupling code logic from PostgreSQL or memory databases.
- **`utils`**: Tracks key-based cooldown windows to prevent thrashing.
- **`metrics`**: Exposes internal operational health (alerts, transitions, plans) via Prometheus.

---

## 2. Webhook intake and Pipeline Sequence

```mermaid
sequenceDiagram
    autonumber
    participant Alertmanager
    participant WebhookHandler as Webhook Handler
    participant AlertParser as Alert Parser
    participant IncidentManager as Incident Manager
    participant PolicyEngine as Policy Engine
    participant CooldownManager as Cooldown Manager
    participant DecisionEngine as Decision Engine
    participant StateMachine as State Machine
    participant RecoveryPlanner as Recovery Planner
    
    Alertmanager->>WebhookHandler: POST /webhook (JSON payload)
    WebhookHandler->>AlertParser: Parse & Validate JSON
    alt Malformed Payload
        AlertParser-->>WebhookHandler: Error
        WebhookHandler-->>Alertmanager: HTTP 400 Bad Request
    else Valid Payload
        AlertParser-->>WebhookHandler: Internal Alert Slice
        WebhookHandler-->>Alertmanager: HTTP 200 OK (Asynchronous handoff)
    end
    
    loop For each alert
        WebhookHandler->>IncidentManager: ProcessAlert(alert, traceID)
        IncidentManager->>StateMachine: Transition (Healthy -> Warning -> Investigating)
        IncidentManager->>PolicyEngine: Match alert to Policy
        IncidentManager->>CooldownManager: Check if in Cooldown window
        IncidentManager->>DecisionEngine: MakeDecision(alert, policy, history, cooldown)
        DecisionEngine-->>IncidentManager: Action Recommendation (e.g. Prepare Rollback)
        IncidentManager->>StateMachine: Transition (Investigating -> DecisionMade)
        
        alt Decision is a recovery action
            IncidentManager->>RecoveryPlanner: Plan(incident, decision, policy)
            RecoveryPlanner-->>IncidentManager: RecoveryPlan (e.g. timeout, priority)
            IncidentManager->>CooldownManager: Record Cooldown
            IncidentManager->>StateMachine: Transition (DecisionMade -> RecoveryPlanned -> Waiting)
        else Decision is Escalate / Ignore / Wait
            IncidentManager->>StateMachine: Transition (DecisionMade -> Waiting/Failed)
        end
    end
```

---

## 3. Finite State Machine (Incident Lifecycle)

```mermaid
stateDiagram-v2
    [*] --> Healthy
    Healthy --> Warning : Alert Firing
    Warning --> Investigating : Operator Starts Diagnostics
    Warning --> Recovered : Alert Resolved Naturally
    
    Investigating --> DecisionMade : Engines Evaluated
    Investigating --> Recovered : Alert Resolved
    
    DecisionMade --> RecoveryPlanned : Plan Generated
    DecisionMade --> Waiting : Wait Decided
    DecisionMade --> Failed : Escalate (Max Retries)
    
    RecoveryPlanned --> Waiting : Plan Dispatched
    
    Waiting --> Recovered : Alert Resolved Successfully
    Waiting --> Failed : Timeout / Max Retries Exceeded
    Waiting --> Investigating : Retry Attempt
    
    Recovered --> Healthy : Incident Closed
    Recovered --> Warning : Alert Re-Fires
    
    Failed --> Warning : Alert Re-Fires
    Failed --> Investigating : Operator Retry
```

*   **Transitions Enforced**: The `StateMachine` checks every change against allowed paths. Attempting to force an illegal state transition immediately rejects the attempt and returns a concrete error.

---

## 4. Class / Component Diagram

```mermaid
classDiagram
    class IncidentStore {
        <<interface>>
        Get(ctx, id) Incident
        GetByFingerprint(ctx, fp) Incident
        Create(ctx, incident)
        Update(ctx, incident)
    }
    class PolicyEngine {
        <<interface>>
        Match(ctx, alert) Policy
    }
    class DecisionEngine {
        <<interface>>
        MakeDecision(ctx, alert, policy, incident, cooldown) DecisionEntry
    }
    class RecoveryPlanner {
        <<interface>>
        Plan(ctx, incident, decision, policy) RecoveryPlan
    }
    class StateMachine {
        <<interface>>
        Transition(ctx, incident, state, reason)
    }
    
    class Manager {
        -IncidentStore store
        -PolicyEngine policyEngine
        -DecisionEngine decisionEngine
        -RecoveryPlanner planner
        -StateMachine stateMachine
        +ProcessAlert(ctx, alert, traceID)
    }
    
    Manager ..> IncidentStore
    Manager ..> PolicyEngine
    Manager ..> DecisionEngine
    Manager ..> RecoveryPlanner
    Manager ..> StateMachine
```
