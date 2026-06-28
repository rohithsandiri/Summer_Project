# Experiment Report: Scenario 4: High Latency Burn Rate Abort

## Scenario Configuration
- **Incident ID**: inc-31b2634c
- **Alert Fingerprint**: alert-s4-latency
- **Target Service**: payment-service
- **Severity Level**: warning
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [07:43:44] Alert Fired: LatencyP95MaxViolation on service payment-service (Severity: warning)
- [07:43:44] State transitioned from Healthy to Warning. Reason: Alert started firing
- [07:43:44] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [07:43:44] Decision Generated: Action=Prepare Rollback, Policy=p-test-4, Reason=Critical burn rate threshold (8.00) exceeded for service payment-service. Initiating immediate automated rollback.
- [07:43:44] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [07:43:44] Recovery Plan Created: ID=plan-payment-service-f232cb59, Action=Prepare Rollback, Cooldown=5s
- [07:43:44] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-payment-service-f232cb59
- [07:43:44] Executing Rollback (Attempt 1/2) on service payment-service
- [07:43:44] State transitioned from RecoveryPlanned to ExecutingRollback. Reason: Initiating Helm Rollback
- [07:43:44] State transitioned from ExecutingRollback to RollbackComplete. Reason: Rollback to revision v1 finished successfully
- [07:43:44] Rollback execution succeeded. Old Revision: 3, Rollback Revision: 1
- [07:43:44] State transitioned from RollbackComplete to Verifying. Reason: Validating system health
- [07:43:44] System health verification check started
- [07:43:44] Verification Successful. System health restored in 40µs.
- [07:43:44] State transitioned from Verifying to Recovered. Reason: System verified healthy
- [07:43:44] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
