# Experiment Report: Scenario 3: Database Connection Loss Rollback

## Scenario Configuration
- **Incident ID**: inc-19aae02e
- **Alert Fingerprint**: alert-s3-db
- **Target Service**: order-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [22:58:06] Alert Fired: DatabaseUnavailable on service order-service (Severity: critical)
- [22:58:06] State transitioned from Healthy to Warning. Reason: Alert started firing
- [22:58:06] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [22:58:06] Decision Generated: Action=Prepare Rollback, Policy=p-test-3, Reason=Critical burn rate threshold (15.00) exceeded for service inventory-service. Initiating immediate automated rollback.
- [22:58:06] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [22:58:06] Recovery Plan Created: ID=plan-inventory-service-9c8928aa, Action=Prepare Rollback, Cooldown=5s
- [22:58:06] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-inventory-service-9c8928aa
- [22:58:06] Recovery execution suppressed — service inventory-service already recovered 4.846918417s ago (within cooldown window)
- [22:58:06] State transitioned from RecoveryPlanned to Waiting. Reason: Recovery suppressed: service already recovered within cooldown window
- [22:58:07] Alert Resolved: DatabaseUnavailable
- [22:58:07] State transitioned from Waiting to Recovered. Reason: Alert resolved by Alertmanager
- [22:58:07] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
