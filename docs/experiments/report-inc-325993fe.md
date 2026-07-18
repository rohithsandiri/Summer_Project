# Experiment Report: Scenario 4: High Latency Burn Rate Abort

## Scenario Configuration
- **Incident ID**: inc-325993fe
- **Alert Fingerprint**: alert-s4-latency
- **Target Service**: payment-service
- **Severity Level**: warning
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [10:25:42] Alert Fired: LatencyP95MaxViolation on service payment-service (Severity: warning)
- [10:25:42] State transitioned from Healthy to Warning. Reason: Alert started firing
- [10:25:42] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [10:25:42] Decision Generated: Action=Prepare Rollback, Policy=p-test-4, Reason=Critical burn rate threshold (8.00) exceeded for service payment-service. Initiating immediate automated rollback.
- [10:25:42] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [10:25:42] Recovery Plan Created: ID=plan-payment-service-a1c345d5, Action=Prepare Rollback, Cooldown=5s
- [10:25:42] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-payment-service-a1c345d5
- [10:25:42] Recovery execution suppressed — service payment-service already recovered 4.544335666s ago (within cooldown window)
- [10:25:42] State transitioned from RecoveryPlanned to Waiting. Reason: Recovery suppressed: service already recovered within cooldown window
- [10:25:42] Alert Resolved: LatencyP95MaxViolation
- [10:25:42] State transitioned from Waiting to Recovered. Reason: Alert resolved by Alertmanager
- [10:25:42] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
