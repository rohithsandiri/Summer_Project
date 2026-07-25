# Experiment Report: Scenario 3: Database Connection Loss Rollback

## Scenario Configuration
- **Incident ID**: inc-6787923f
- **Alert Fingerprint**: alert-s3-db
- **Target Service**: order-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [20:48:57] Alert Fired: DatabaseUnavailable on service order-service (Severity: critical)
- [20:48:57] State transitioned from Healthy to Warning. Reason: Alert started firing
- [20:48:57] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [20:48:57] Decision Generated: Action=Prepare Rollback, Policy=p-test-3, Reason=Critical burn rate threshold (8.00) exceeded for service payment-service. Initiating immediate automated rollback.
- [20:48:57] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [20:48:57] Recovery Plan Created: ID=plan-payment-service-1aa27515, Action=Prepare Rollback, Cooldown=5s
- [20:48:57] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-payment-service-1aa27515
- [20:48:57] Recovery execution suppressed — service payment-service already recovered 4.697263292s ago (within cooldown window)
- [20:48:57] State transitioned from RecoveryPlanned to Waiting. Reason: Recovery suppressed: service already recovered within cooldown window
- [20:48:58] Alert Resolved: DatabaseUnavailable
- [20:48:58] State transitioned from Waiting to Recovered. Reason: Alert resolved by Alertmanager
- [20:48:58] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
