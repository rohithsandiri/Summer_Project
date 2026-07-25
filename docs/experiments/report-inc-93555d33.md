# Experiment Report: Scenario 1: Canary Deployment Guard Rollback

## Scenario Configuration
- **Incident ID**: inc-93555d33
- **Alert Fingerprint**: alert-s1-canary
- **Target Service**: payment-service
- **Severity Level**: critical
- **Execution Mode**: Automated Self-Healing Control Plane

## Timeline of Events
- [20:54:46] Alert Fired: HighErrorRate on service payment-service (Severity: critical)
- [20:54:46] State transitioned from Healthy to Warning. Reason: Alert started firing
- [20:54:46] State transitioned from Warning to Investigating. Reason: Operator evaluating alert policies
- [20:54:46] Decision Generated: Action=Prepare Rollback, Policy=p-test-1, Reason=Critical burn rate threshold (8.00) exceeded for service payment-service. Initiating immediate automated rollback.
- [20:54:46] State transitioned from Investigating to DecisionMade. Reason: Decision completed: Prepare Rollback
- [20:54:46] Recovery Plan Created: ID=plan-payment-service-5feaf229, Action=Prepare Rollback, Cooldown=5s
- [20:54:46] State transitioned from DecisionMade to RecoveryPlanned. Reason: Recovery plan created: plan-payment-service-5feaf229
- [20:54:46] Executing Rollback (Attempt 1/2) on service payment-service
- [20:54:46] State transitioned from RecoveryPlanned to ExecutingRollback. Reason: Initiating Helm Rollback
- [20:54:46] State transitioned from ExecutingRollback to RollbackComplete. Reason: Rollback to revision v1 finished successfully
- [20:54:46] Rollback execution succeeded. Old Revision: 2, Rollback Revision: 1
- [20:54:46] State transitioned from RollbackComplete to Verifying. Reason: Validating system health
- [20:54:46] System health verification check started
- [20:54:46] Verification Successful. System health restored in 183µs.
- [20:54:46] State transitioned from Verifying to Recovered. Reason: System verified healthy
- [20:54:46] State transitioned from Recovered to Healthy. Reason: Incident resolved and archived


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
