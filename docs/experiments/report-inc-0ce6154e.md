# Experiment Report: Scenario 3: Database Connection Loss Rollback

## Scenario Configuration
- **Incident ID**: inc-0ce6154e
- **Alert Fingerprint**: alert-s3-db
- **Target Service**: order-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [23:00:54] Alert Fired: DatabaseUnavailable on service order-service (Severity: critical)
- [23:00:54] State transitioned from Healthy to Warning. Reason: Alert started firing
- [23:00:54] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [23:00:54] Decision Generated: Action=Prepare Rollback, Policy=p-test-3, Reason=Critical burn rate threshold (15.00) exceeded for service inventory-service. Initiating immediate automated rollback.
- [23:00:54] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [23:00:54] Recovery Plan Created: ID=plan-inventory-service-e8d7b13e, Action=Prepare Rollback, Cooldown=5s
- [23:00:54] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-inventory-service-e8d7b13e
- [23:00:54] Recovery execution suppressed — service inventory-service already recovered 4.848339542s ago (within cooldown window)
- [23:00:54] State transitioned from RecoveryPlanned to Waiting. Reason: Recovery suppressed: service already recovered within cooldown window
- [23:00:54] Alert Resolved: DatabaseUnavailable
- [23:00:54] State transitioned from Waiting to Recovered. Reason: Alert resolved by Alertmanager
- [23:00:54] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
