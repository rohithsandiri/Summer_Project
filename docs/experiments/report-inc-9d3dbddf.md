# Experiment Report: Scenario 4: High Latency Burn Rate Abort

## Scenario Configuration
- **Incident ID**: inc-9d3dbddf
- **Alert Fingerprint**: alert-s4-latency
- **Target Service**: payment-service
- **Severity Level**: warning
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [23:02:59] Alert Fired: LatencyP95MaxViolation on service payment-service (Severity: warning)
- [23:02:59] State transitioned from Healthy to Warning. Reason: Alert started firing
- [23:02:59] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [23:02:59] Decision Generated: Action=Prepare Rollback, Policy=p-test-4, Reason=Critical burn rate threshold (8.00) exceeded for service payment-service. Initiating immediate automated rollback.
- [23:02:59] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [23:02:59] Recovery Plan Created: ID=plan-payment-service-1bd62db4, Action=Prepare Rollback, Cooldown=5s
- [23:02:59] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-payment-service-1bd62db4
- [23:02:59] Recovery execution suppressed — service payment-service already recovered 4.537550833s ago (within cooldown window)
- [23:02:59] State transitioned from RecoveryPlanned to Waiting. Reason: Recovery suppressed: service already recovered within cooldown window
- [23:02:59] Alert Resolved: LatencyP95MaxViolation
- [23:02:59] State transitioned from Waiting to Recovered. Reason: Alert resolved by Alertmanager
- [23:02:59] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
