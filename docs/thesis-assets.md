# Thesis Assets: Self-Healing Control Plane Operator

This document compiles the academic assets, problem statement, and formal research findings suitable for the M.Tech thesis submission.

---

## 1. Abstract
As organizations transition to microservice architectures running on Kubernetes, maintaining high availability and meeting Service Level Objectives (SLOs) becomes increasingly complex. Traditional incident response relies on human Operators responding to alerts, leading to elevated Mean Time to Recovery (MTTR) and high cognitive load. This thesis presents a **Self-Healing Kubernetes Control Plane Operator** that automates microservice recovery by integrating Prometheus metrics monitoring with programmatic Helm SDK rollbacks. 

The operator continuously monitors Service Level Indicators (SLIs), evaluates error budget burn rates, identifies root-cause services using dependency graphs, and makes recovery decisions. Experimental results demonstrate that the automated recovery plane reduces MTTR from **14+ minutes (manual)** to **~76 seconds (automated)**, representing an 11x improvement while ensuring zero-downtime deployment safety via active deployment guards.

---

## 2. Problem Statement
Modern cloud-native environments run hundreds of interconnected deployments. Software upgrades often introduce regressions (e.g. latency spikes, memory leaks, and high error rates). 

Key challenges include:
1. **High MTTR**: Relying on manual developer intervention to find the bug, look up the repository revision, and run rollback commands.
2. **Cascading Failures**: Lack of real-time dependency analysis causes upstream services to choke.
3. **Budget Exhaustion**: Deploying new releases when the system's error budget is already depleted.

---

## 3. Research Objectives
1. Design a custom Kubernetes Operator capable of executing programmatic Helm SDK rollbacks in response to SLO breaches.
2. Implement SRE control engines to analyze SLOs, error budget burn rates, and service dependency trees.
3. Design a Deployment Guard to automatically block progressive canary rollouts when the target service has depleted its error budget.
4. Benchmark the automated MTTR improvement factor against manual operator workflows.

---

## 4. Evaluation & Results
1. **MTTR Reduction**: The platform consistently restored services within `90 seconds` from the start of an SLO alert, compared to `10-30 minutes` for manual rollback.
2. **Stability**: Cooldown policies successfully prevented "flapping" (repetitive rolling back when pods are initializing).
3. **Zero Budget Overruns**: Deployment Guard protected staging/production environments by blocking rollouts when burn rates exceeded `14.4`.
