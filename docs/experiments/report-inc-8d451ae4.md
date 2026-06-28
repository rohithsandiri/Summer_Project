# Experiment Report: Scenario 2: Inventory Crash Mitigation

## Scenario Configuration
- **Incident ID**: inc-8d451ae4
- **Alert Fingerprint**: alert-s2-crash
- **Target Service**: inventory-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [07:11:01] Alert Fired: InventoryServiceDown on service inventory-service (Severity: critical)
- [07:11:01] State transitioned from Healthy to Warning. Reason: Alert started firing
- [07:11:01] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [07:11:01] Decision Generated: Action=Prepare Rollback, Policy=p-test-2, Reason=Critical burn rate threshold (15.00) exceeded for service inventory-service. Initiating immediate automated rollback.
- [07:11:01] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [07:11:01] Recovery Plan Created: ID=plan-inventory-service-8cb84e0b, Action=Prepare Rollback, Cooldown=5s
- [07:11:01] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-inventory-service-8cb84e0b
- [07:11:01] Executing Rollback (Attempt 1/2) on service inventory-service
- [07:11:01] State transitioned from RecoveryPlanned to ExecutingRollback. Reason: Initiating Helm Rollback
- [07:11:01] State transitioned from ExecutingRollback to RollbackComplete. Reason: Rollback to revision v1 finished successfully
- [07:11:01] Rollback execution succeeded. Old Revision: 2, Rollback Revision: 1
- [07:11:01] State transitioned from RollbackComplete to Verifying. Reason: Validating system health
- [07:11:01] System health verification check started
- [07:11:01] Verification Successful. System health restored in 47µs.
- [07:11:01] State transitioned from Verifying to Recovered. Reason: System verified healthy
- [07:11:01] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


## Metrics Snapshot (Before Recovery)
- **Error Rate**: Burn Rate: 0.00, Remaining Error Budget: 0.00%
- **SLO Metrics**: Golden signals normalized, error rate returned to <0.01%, latency <300ms

## Operator Policy Decisions
- **Decision Engine Output**: 
- **Recommended Action**: 
- **Helm Rollback Target Revision**: 0
- **Verification Status**: 
- **Total Recovery Duration**: 0.00 seconds

## Post-Mortem & Lessons Learned
System successfully recovered using automated Helm rollback. Ensure upstream canary dependencies have updated policies configured to prevent budget exhaustions.
