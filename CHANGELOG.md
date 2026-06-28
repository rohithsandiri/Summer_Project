# Changelog

All notable changes to the Self-Healing Platform will be documented in this file.

## [1.0.0] - 2026-06-28
### Added
- **Operator High Availability**: Active-Standby coordination lease-locking via Kubernetes Coordination API.
- **Security Hardening**: Workload seccomp profiles, read-only root filesystems, dropped capabilities, and non-root runtime environments.
- **Kubernetes Scheduling Safety**: Custom PriorityClasses, pod anti-affinities, and TopologySpreadConstraints templates.
- **Backup & Restore Utility**: Shell script backing up Postgres DB dumps, Helm release secrets, and active incident states.
- **Continuous Soak & Chaos Suite**: Real-time memory leak checks, pod eviction scenarios, and load generators.
- **OpenTelemetry & Distributed Tracing**: Auto-injected tracing across API Gateway and microservice requests.
- **SRE SLO Control Engines**: Burn rate evaluation, budget estimation, root cause identification, and dependency graphing.
- **Progressive Delivery & Deployment Guard**: Canary deployment validations via Argo Rollouts.
- **Performance Profiling**: Registered pprof debugging routes on all service serve muxes.

### Changed
- Refactored all loggers to use structured fields.
- Formatted Go sources to meet strict standards.
- Strengthened health status probes.
