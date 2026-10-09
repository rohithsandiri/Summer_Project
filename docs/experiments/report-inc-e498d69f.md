# Experiment Report: Scenario 2: Inventory Crash Mitigation

## Scenario Configuration
- **Incident ID**: inc-e498d69f
- **Alert Fingerprint**: alert-s2-crash
- **Target Service**: inventory-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [23:03:19] Alert Fired: InventoryServiceDown on service inventory-service (Severity: critical)
- [23:03:19] State transitioned from Healthy to Warning. Reason: Alert started firing
- [23:03:19] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [23:03:19] Decision Generated: Action=Prepare Rollback, Policy=p-test-2, Reason=Critical burn rate threshold (15.00) exceeded for service inventory-service. Initiating immediate automated rollback.
- [23:03:19] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [23:03:19] Recovery Plan Created: ID=plan-inventory-service-0086ac93, Action=Prepare Rollback, Cooldown=5s
- [23:03:19] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-inventory-service-0086ac93
- [23:03:19] Executing Rollback (Attempt 1/2) on service inventory-service
- [23:03:19] State transitioned from RecoveryPlanned to ExecutingRollback. Reason: Initiating Helm Rollback
- [23:03:19] State transitioned from ExecutingRollback to RollbackComplete. Reason: Rollback to revision v1 finished successfully
- [23:03:19] Rollback execution succeeded. Old Revision: 2, Rollback Revision: 1
- [23:03:19] State transitioned from RollbackComplete to Verifying. Reason: Validating system health
- [23:03:19] System health verification check started
- [23:03:19] Verification Successful. System health restored in 748µs.
- [23:03:19] State transitioned from Verifying to Recovered. Reason: System verified healthy
- [23:03:19] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
