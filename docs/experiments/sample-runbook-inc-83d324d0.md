# Sample Auto-Generated Runbook (live incident, dependency batch rollback test)

Fetched from the operator at `GET /runbooks?incident_id=inc-83d324d0&format=markdown` on 2026-07-18.

---

# SRE Automated Runbook: Incident inc-83d324d0

## Summary
Automated recovery runbook generated for active warning/critical incident on service order-service.

## Root Cause Analysis
High SLO violation detected (Availability < 99.9%).

## Metrics Snapshot (Before Recovery)
Burn Rate: 0.00, Remaining Error Budget: 0.00%

## Recovery Actions Executed
* Helm Rollback triggered on release: order-service
* Restored revision from 23 to stable revision 22

## Verification Details
Verification succeeded. Pods ready and SLO compliance restored.

## Recommendations
* Observe metrics for the next 15 minutes to confirm stability.
* Increase CPU/Memory resource limit profiles if OOMKilled pattern is identified.
* Update deployment configuration with tighter readiness probe thresholds.

## Incident Timeline (from /timelines)

```json
[
    {
        [1m"id"[0m: [32m"tl-1784373613291317419"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.291415044Z"[0m,
        [1m"message"[0m: [32m"Alert Fired: OrderServiceHighErrorRate on service order-service (Severity: critical)"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613292124378"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.292125753Z"[0m,
        [1m"message"[0m: [32m"State transitioned from Healthy to Warning. Reason: Alert started firing"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613292808503"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.292809628Z"[0m,
        [1m"message"[0m: [32m"State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613297835336"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.297836586Z"[0m,
        [1m"message"[0m: [32m"Decision Generated: Action=Prepare Rollback, Policy=policy-order-error-rate-rollback, Reason=Policy matched: policy-order-error-rate-rollback. Recommending action: Prepare Rollback"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613297849044"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.297849419Z"[0m,
        [1m"message"[0m: [32m"State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613298034211"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
        [1m"timestamp"[0m: [32m"2026-07-18T11:20:13.298034753Z"[0m,
        [1m"message"[0m: [32m"Recovery Plan Created: ID=plan-order-service-bdf84f64, Action=Prepare Rollback, Cooldown=5m0s"[0m
    },
    {
        [1m"id"[0m: [32m"tl-1784373613298040086"[0m,
        [1m"incident_id"[0m: [32m"inc-83d324d0"[0m,
```
