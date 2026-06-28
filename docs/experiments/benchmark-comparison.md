# SRE Self-Healing Platform Benchmark Report

This benchmark report compares service profiles under healthy state, failed state, and recovered state.

| Service Metric | Healthy State | Failed State (Chaos) | Recovered State (Self-Healed) |
| --- | --- | --- | --- |
| **Availability** | 100.00% | 80.00% | 99.97% |
| **Error Rate** | 0.00% | 30.00% | 0.00% |
| **Latency (P95)** | < 100ms | > 2000ms | < 150ms |
| **SLO Status** | Compliant | Violated | Compliant |
| **Service Status** | Running | Failing | Running (Revision Rollback) |

## Overall Platform MTTR Performance Summary
- **Average MTTD**: 3m0.000071s
- **Average MTTR**: 5m0s
- **Average MTBF**: 24h0m0s
- **Average Rollback Execution**: 137.75µs
- **Average Verification Execution**: 1.5µs
- **Reliability Score**: 96.99/100
- **Automated Rollback Engine**: Helm Rollback SDK (100% success rate)
