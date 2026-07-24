# Live Failure-Simulation Findings — SLO-Driven Self-Healing Verification

**Date:** 2026-07-18
**Environment:** minikube (Docker driver, Kubernetes v1.35.1), kube-prometheus-stack in `monitoring`, workloads + rollback-operator in `default`
**Traffic:** in-cluster generator POSTing ~3 orders/sec to order-service (which fans out to inventory-service and payment-service)
**Method:** every failure injected through `helm upgrade --set …` (reproducible), detected by a specific PrometheusRule alert, recovered by the rollback-operator, MTTR measured from injection timestamp to operator-initiated Helm rollback.

All timestamps below are local (UTC+5:30).

---

## 1. Executive Summary

| # | Scenario | Injection | Detecting alert | Expected MTTR | Measured MTTR | Verdict |
|---|----------|-----------|-----------------|---------------|---------------|---------|
| S1 | High 5xx error rate (50%) | `order-service --set errorRate=0.5` | `OrderServiceHighErrorRate` | 2–4 min | **137 s** (2 m 17 s) | ✅ Pass |
| S2 | P99 latency spike (3 s/request) | `inventory-service --set latencyMs=3000` | `InventoryServiceHighLatency` | 3–5 min | **196 s** (3 m 16 s) | ✅ Pass — but exposed Finding F1/F2 |
| S3 | Error burst vs strict 1 % SLO (30%) | `payment-service --set errorRate=0.3` | `PaymentServiceHighErrorRate` | 2–3 min | **139 s** (2 m 19 s) | ✅ Pass |
| S4 | Combined 20 % errors + 1 s latency | `order-service --set errorRate=0.2 --set latencyMs=1000` | `OrderServiceHighErrorRate` / `HighLatency` | 2–4 min | > 400 s (contaminated) | ⚠️ Blocked by residual cooldown from F1 thrashing |
| S5 | CrashLoopBackOff via startup panic | `order-service --set errorRate=5.0` (fails config validation → `log.Fatalf`) | *(none routed)* | 2–3 min | n/a — zero user impact | ✅ Contained by `maxUnavailable: 0`; gap noted as F4 |

The first rollback in every SLO scenario landed inside the expected MTTR window. However, the suite uncovered **three real defects** (F1–F3) that caused **rollback oscillation** — services being "rolled back" onto the faulty revision they had just escaped — plus one detection gap (F4). F1–F3 were fixed, the fix was redeployed, and the oscillation scenario was re-run to confirm stable single-rollback behavior (section 5).

---

## 2. Scenario Timelines

### S1 — High 5xx Error Rate (order-service, 50 %)

| Time | Event |
|------|-------|
| 13:11:37 | `helm upgrade order-service --set errorRate=0.5` → revision 4 live |
| ~13:12–13:13 | Prometheus `OrderServiceHighErrorRate` breaches, sustains through `for: 1m` |
| 13:13:49 | Alertmanager webhook hits operator; policy `policy-order-error-rate-rollback` matched; state machine walks Warning → Investigating → DecisionMade → RecoveryPlanned → ExecutingRollback |
| 13:13:50 | `helm rollback` to revision 3 complete (revision 5, "Rollback to 3") |
| 13:14:02 | Verification passed after waiting out pod rollout; incident Recovered → Healthy |

**MTTR: 137 s** injection → rollback; 145 s to verified-healthy. Detection dominates (~132 s of the 137 s); the operator itself decides and executes in ~1 s.

### S2 — P99 Latency Spike (inventory-service, 3 000 ms)

| Time | Event |
|------|-------|
| 13:18:16 | `helm upgrade inventory-service --set latencyMs=3000` → revision 4 |
| 13:21:19–13:21:32 | `InventoryServiceHighLatency` fires; operator rolls back to revision 3 |

**MTTR: 196 s.** Within the 3–5 min expectation (latency rules carry `for: 2m` + a 5-minute rate window, so detection is inherently slower than error-rate).

**Cascade observed (expected and correct):** order-service calls inventory-service synchronously, so the injected 3 s latency propagated upstream — `OrderServiceHighLatency` fired too and the operator rolled order-service back as well. With a dependency-blind operator this is a no-fault rollback of an innocent service; it is harmless here (rollback re-applied an equivalent config) but is worth noting for the dependency-graph roadmap.

**What went wrong next is Finding F1/F2 (below):** the latency alert was firing once *per pod*, spawning multiple concurrent incidents that raced each other into repeated rollbacks.

### S3 — Strict-SLO Error Burst (payment-service, 30 % vs 1 % SLO)

| Time | Event |
|------|-------|
| 13:23:34 | `helm upgrade payment-service --set errorRate=0.3` → revision 4 |
| 13:25:53 | Operator rollback to revision 3 detected |

**MTTR: 139 s.** No oscillation for payment-service — its error-rate alert aggregates with `sum()` into a single series, so only one incident existed. This asymmetry versus S2 is what pinpointed Finding F2.

### S4 — Combined Failure (order-service, 20 % errors + 1 s latency)

Injected 13:27:54 (revision 9). No rollback within the 400 s watch window. Root cause: order-service was still inside the per-alert cooldown from the F1 thrashing that immediately preceded the scenario, so the decision engine correctly answered `Wait` ("Service is in a cooldown window to prevent thrashing"). A rollback did land later, but targeted a faulty revision (F3). Verdict: the cooldown logic *worked as designed*; the scenario result is contaminated by F1 and was superseded by the re-verification run in section 5.

### S5 — CrashLoopBackOff (order-service, invalid `ERROR_RATE=5.0`)

The services validate config at startup and `log.Fatalf` on out-of-range values, so `--set errorRate=5.0` produces a genuine CrashLoopBackOff — a pure-Helm reproduction of the "bad env var causes panic" injection.

| Time | Event |
|------|-------|
| 13:36:49 | `helm upgrade order-service --set errorRate=5.0` → revision 11; new pod enters CrashLoopBackOff (2 restarts within 29 s) |
| throughout | Old ReplicaSet's 2 pods keep serving 100 % of traffic — `maxUnavailable: 0` prevents the rollout from touching them |

**Result: zero user impact.** `OrderServiceNoHealthyPods` (available == 0) correctly never fired because availability never dropped. The deployment strategy contained the blast radius exactly as designed — but nothing rolls the stuck revision back either (Finding F4). The kube-prometheus-stack's own `KubePodCrashLooping` alert is warning-severity with `for: 15m`, so it routes to the null receiver, never the operator.

---

## 3. Findings (Defects Discovered)

### F1 — Rollback oscillation: recoveries re-applied the faulty revision  **[FIXED]**

After a successful rollback, the faulty revision becomes the newest *superseded* entry in Helm history. When a second recovery fired for the same service (from duplicate incidents — F2 — or a retry racing a concurrent rollback), target selection picked "newest superseded revision below current", i.e. **the faulty revision the previous rollback had just escaped**. Helm history recorded the ping-pong directly: `Rollback to 3` (good) → `Rollback to 4` (faulty) → `Rollback to 5` (good) → `Rollback to 6` (faulty). At suite end, order-service was live with `errorRate: 0.2, latencyMs: 1000` and inventory-service with `latencyMs: 3000` — the platform reported successful recoveries while actually serving the injected faults.

Contributing factor: post-rollback verification checks only Kubernetes pod health (chaos-injected pods still answer `/health` with 200), so a rollback onto a faulty revision still "verifies" successfully.

**Fix:** `GetLastHealthyRevision` now skips any revision whose user-supplied values (`rel.Config`) deep-equal the current (faulty) revision's values — rolling back to an identical config is by definition a no-op that leaves the fault in place.

### F2 — Per-pod latency alerts spawned duplicate concurrent incidents  **[FIXED]**

`histogram_quantile(0.99, rate(…_bucket[5m]))` without aggregation evaluates per scraped series, so a 2-replica service breaching its latency SLO fired **two alert instances with distinct fingerprints** → two independent incidents → two racing recovery pipelines (the operator log shows both determining "Current: v5, Target: v4" within the same second; one then hit Helm's "operation in progress" lock, classified it transient, and re-rolled-back a minute later — undoing the config the other pipeline had just restored). Error-rate alerts were immune because they aggregate with `sum()`.

**Fix:** latency rules now aggregate buckets before the quantile: `histogram_quantile(0.99, sum by (le) (rate(…_bucket[5m])))` — one series, one alert, one incident per service.

### F3 — Cooldown was advisory, checked non-atomically, and bypassed by retries  **[FIXED]**

The cooldown check ran at *decision* time: concurrent incidents for the same service could all pass `IsCoolingDown` before any of them recorded a recovery, and the retry engine's exponential backoff (1 min) re-executed rollbacks well inside the 5–10 min service cooldown. Once alert metric windows are stale (a `[5m]` rate window keeps a resolved breach "visible" for minutes), those extra executions are pure thrash.

**Fix:** an atomic per-service execution gate (`CooldownManager.TryStartExecution`) — check-and-claim under a single lock, keyed by service (not alert), enforced at the top of the recovery execution pipeline. Exactly one rollback per service per cooldown window can execute; suppressed pipelines transition their incident to `Waiting` and log the suppression. Verified with a 50-goroutine race test (`-race`).

### F4 — Stuck CrashLoop rollout is contained but never rolled back  **[GAP — recommendation]**

With `maxUnavailable: 0`, a crash-looping rollout strands the new ReplicaSet while old pods serve traffic: no SLO breaches, no critical alert, no operator involvement — the bad revision sits in "deployed" state indefinitely and the *next* legitimate deploy would inherit the fault. Recommendation: add a critical-severity stuck-rollout rule, e.g.

```promql
kube_deployment_status_condition{condition="Progressing", status="false"} == 1
```

or `kube_deployment_status_replicas_updated < kube_deployment_spec_replicas` sustained `for: 5m`, labeled with `helm_release`/`namespace` and mapped to a rollback policy, so the operator also heals deployments that Kubernetes has quarantined but cannot fix.

---

## 4. Fixes Applied

| File | Change |
|------|--------|
| `internal/operator/helm/manager.go` | Rollback target selection skips revisions whose user-supplied values equal the current faulty revision's values (F1) |
| `k8s/prometheus-rules/slo-alerting-rules.yaml` | Latency alerts aggregate histogram buckets with `sum by (le)` before `histogram_quantile` (F2) |
| `internal/operator/utils/cooldown.go` | New `TryStartExecution`: atomic per-service check-and-claim execution gate (F3) |
| `internal/operator/interfaces/interfaces.go` | `CooldownManager` interface extended with `TryStartExecution` |
| `internal/operator/incident/manager.go` | Recovery pipeline acquires the execution gate before executing; suppressed duplicates transition to `Waiting` (F3) |
| `internal/operator/utils/cooldown_test.go` | Race-detector-verified tests: single winner among 50 racers, per-service independence, window expiry |

All unit tests pass (`go test ./internal/operator/... -race`). The operator image was rebuilt and redeployed to the cluster.

---

## 5. Re-Verification After Fixes

The oscillation-provoking scenario (S2, `inventory-service --set latencyMs=3000`) was re-run against the rebuilt operator, followed by an 8-minute stability watch — the original run began oscillating within ~90 seconds of its first rollback.

| Time | Event |
|------|-------|
| 13:46:59 | Injected `latencyMs=3000` → revision 10 |
| 13:52:03 | Operator rollback to revision 9 (the clean baseline) — **MTTR 304 s** |
| 13:52:03–14:00:18 | **Stability watch: no further rollbacks.** Values stayed clean (`latencyMs: 0`); incident closed Recovered → Healthy once |

**Oscillation eliminated.** Helm history tells the whole story in one screen: the pre-fix suite left `Rollback to 3 → Rollback to 4 → Rollback to 5 → Rollback to 6` (revisions 5–8, ping-ponging between good and faulty), while the post-fix run shows a single `Rollback to 9` followed by silence.

The MTTR rose from 196 s to 304 s: the pre-fix run was detected by whichever *single pod's* p99 crossed first, while the aggregated rule (F2 fix) requires the service-wide p99 to cross. Slightly slower detection is the honest price of one-alert-per-service; ~5 minutes is still within a reasonable band for a `for: 2m` rule over a 5-minute rate window.

**The suppression gate fired in production conditions.** During the re-run, the latency cascade rolled order-service back as well, and two minutes later a second recovery for order-service was attempted by a different incident — the gate suppressed it:

```
WARN recovery execution suppressed: another recovery for this service ran
     recently  Service=order-service  remaining_window=2m49s
INFO incident state transitioned  from_state=RecoveryPlanned  to_state=Waiting
```

**Residual finding (F5):** that cascade rollback moved order-service from an already-clean revision onto a stale faulty one (target selection cannot know a config is faulty without consulting metrics — F1's config-skip only guarantees the target *differs* from the current config). The gate then correctly held further rollbacks, and the suppressed incident sat in `Waiting` — but nothing re-triggers it internally: re-processing depends on Alertmanager re-delivering the alert, which the route's `repeatInterval: 30m` delays. The system converges, but with up to ~30 minutes of exposure. We restored order-service manually to conclude the experiment. See F5's recommendation below.

### F5 — Suppressed recoveries wait on Alertmanager's re-notification cadence  **[GAP — recommendation]**

When the execution gate suppresses a duplicate recovery, the incident parks in `Waiting` with no internal timer. If the suppressed recovery was actually needed (as after F5's wrong-direction cascade rollback), the service stays degraded until Alertmanager re-notifies — bounded by `repeatInterval` (30 min on the critical route). Two complementary remedies:

1. **Reconcile loop in the operator:** periodically re-evaluate `Waiting` incidents whose service's gate has expired, re-querying Prometheus for the violated SLO before acting (the FSM already permits `Waiting → Investigating`).
2. **SLO-aware verification (also closes F1's root):** after any rollback, verify against the *violated metric* via the operator's existing Prometheus client, not just pod health. A rollback that lands on a faulty config would then fail verification and walk further back through history instead of reporting success.

---

## 6. Part 2 — Enhancement Implementation and Live Verification

A second implementation wave added the four "make it smarter" capabilities, each verified live. The live tests caught **three additional bugs** (F6–F8) that unit tests and the first suite had missed.

### 6.1 CrashLoopBackOff auto-rollback (closes F4)

New critical `*StuckRollout` rules detect a rollout whose updated replicas stay below spec for 3 minutes (the signature of CrashLoopBackOff/image failures contained by `maxUnavailable: 0`), mapped to rollback policies.

**Live test:** `helm upgrade order-service --set errorRate=5.0` — the out-of-range value fails config validation, the service `log.Fatalf`s at startup, and the new pod enters a genuine CrashLoopBackOff while old pods keep serving (zero user impact throughout).

| Time | Event |
|---|---|
| 16:14:57 | Injected (revision 18); new pod crash-looping within 30 s |
| 16:18:57≈ | `OrderServiceStuckRollout` fired (3 m `for:` + evaluation) |
| 16:19:12 | Operator matched `policy-order-stuck-rollout-rollback`, rolled back |
| 16:19:17 | **Auto-rollback confirmed — MTTR 260 s**, both pods healthy |

The dependency coordinator also ran and correctly declined both peers ("deploy_gap=2h22m … not part of deployment batch").

### 6.2 New findings from the live tests (all fixed)

**F6 — Alert parser preferred the scrape target's `service` label over `helm_release`.** The first stuck-rollout attempt fired and routed perfectly, but the operator logged `Service=monitoring-kube-state-metrics` — kube-state-metrics' own `service` label rode along on the alert, no policy matched, decision was Ignore. Every alert built on kube-state-metrics series (including the pre-existing `NoHealthyPods` rules) had this latent bug. Fix: `helm_release` — the label our rules set deliberately as the operator contract — now takes precedence in the parser.

**F7 — Helm history arrives lexicographically sorted, so rollback targets were near-arbitrary past 10 revisions.** The SDK's history action returns releases in Kubernetes secret-name order (`…v1, v10, …, v18, v2, …, v9`), so "newest revision at the end" silently broke once a release accumulated 10+ revisions — the backward walk repeatedly picked `v9` (observed from both v13 and v18), which retroactively explains the wrong-direction cascade rollback in Part 1's re-verification. Fix: sort history numerically by revision before target selection.

**F8 — SLO verification pointed at a non-existent Prometheus hostname.** The default `prometheus-k8s.monitoring.svc` is the plain kube-prometheus manifests' service name; kube-prometheus-stack names it `monitoring-kube-prometheus-prometheus`. The graceful fallback (pod-health only, with a warning) worked exactly as designed, which is why this was a log-line discovery rather than an outage. Fix: correct default plus explicit `PROMETHEUS_ADDR` in the operator chart.

**Operational note (reinforces F5):** during the enhancement rollout, Alertmanager delivered a firing notification to the old operator pod as it terminated, recorded the delivery as successful, and deduplicated further notifications for the group until `repeatInterval`. Rolling the operator mid-incident can therefore eat one notification — an internal reconcile loop (F5 recommendation 1) is the durable answer; restarting Alertmanager (state is on emptyDir) is the operational workaround.

### 6.3 Dependency-aware batch rollback

`internal/operator/dependency/coordinator.go`: after a primary rollback succeeds, direct callers and callees of the failed service whose current revision was deployed within the batch window (default 10 min, `DEPENDENCY_BATCH_WINDOW_MINUTES`) are rolled back too — co-released services share API contracts, and rolling back only the callee strands its callers on endpoints that no longer exist. Peers claim the same per-service execution gate as alert-driven recoveries (no double rollbacks, no recursion; transitive closure deliberately excluded). Unit-tested (co-release, out-of-window, gate-claimed, initial-revision cases) and verified live in the batch test below.

**Live test — the caller/callee co-release scenario:** inventory-service (benign change) and order-service (`errorRate=0.5`) deployed as one batch, 1 second apart.

| Time | Event |
|---|---|
| 16:48:08 | Batch deployed: inventory rev 14 (benign), order rev 23 (faulty) |
| 16:50:13 | `OrderServiceHighErrorRate` fired → order rolled back v23 → v22 (correct clean target — F7's numeric sort verified live) — **MTTR 126 s** |
| 16:50–16:52 | SLO-aware verification held the incident in Verifying until the alert actually cleared in Prometheus: "slo verification passed: triggering alert no longer firing" at 16:52:44 |
| 16:52:44 | Dependency coordinator: inventory deploy gap **299 ms** → co-released → rolled back rev 14 → 13; payment deploy gap 3 h 7 m → correctly left alone |
| 16:53:19 | Final state: both services on clean configs (`errorRate 0, latencyMs 0`), incident Recovered → Healthy |

One incident, one alert, and the operator restored the *entire release batch* — the caller was never left calling an API contract its callee no longer served.

### 6.4 SLO-aware verification (closes F1-root and most of F5)

The verification loop now requires two signals before declaring Recovered: pod health (as before) **and** the triggering alert no longer firing in Prometheus (`ALERTS{alertname=…, alertstate="firing"}` via the operator's Prometheus client). A rollback that lands on a still-faulty revision now fails verification and walks further back through history within the same incident, instead of reporting a false success. Prometheus being unreachable degrades gracefully to pod-health-only with a logged warning (F8 proved this path live).

### 6.5 Automatic runbook generation

Already implemented in the reliability engine and exposed at `GET /runbooks?incident_id=<id>&format=markdown|json|html` — root cause, full timeline, recovery actions, Helm revisions, verification results, MTTR. Verified live against a real incident (sample in `docs/experiments/`).

### 6.6 Observability

- **Distributed tracing:** Jaeger all-in-one deployed (`k8s/monitoring/jaeger.yaml`); service charts set `OTEL_EXPORTER_OTLP_ENDPOINT=jaeger:4318`; order-service and inventory-service traces confirmed arriving in the Jaeger API during the tests.
- **Monitoring guide:** `docs/monitoring-guide.md` — every dashboard, UI, API endpoint, and port-forward, plus the honest trade-off on centralized logging at this cluster's memory budget.
- **Automated simulations:** `scripts/05-run-failure-suite.sh` runs all five scenarios non-interactively, measures MTTR, verifies rollbacks landed on clean configs, restores on failure, and writes a timestamped markdown report — cron-schedulable.

## 6.7 Self-maintaining dependency graph (replaces the static graph)

The dependency graph used for batch rollback (§6.3) was originally hand-written in code (`dependency.NewGraph()`: three `AddEdge` calls). That's a maintenance liability — a new service or a new call path silently doesn't participate in batch rollback until someone edits the graph and redeploys the operator. This was replaced with a **self-maintaining, trace-derived graph**.

**Approach survey.** Production root-cause-analysis systems solve "what calls what" the same way: build the graph from observed traffic rather than declare it.

- **Trace-based construction** — Microscope and MicroHECL (used in production at Alibaba) build their service call graph dynamically from distributed traces / network-connection data each analysis cycle, rather than from a static topology file ([Microscope, ICSOC 2018](https://link.springer.com/chapter/10.1007/978-3-030-03596-9_1); [MicroHECL, ICSE-SEIP 2021](https://arxiv.org/abs/2103.01782)). A broader OpenTelemetry-trace-based pipeline using this technique for RCA reports 60–78% faster root-cause identification than manual graphs ([survey discussion](https://arxiv.org/html/2408.00803v1)).
- **eBPF / kernel-level discovery** (Cilium Hubble, Pixie) — zero-instrumentation service maps built from every observed TCP connection at the kernel, no app-level tracing required ([Cilium service map](https://cilium.io/use-cases/service-map/)).
- **Static manifest analysis** — infer edges from Deployment env vars / K8s NetworkPolicies at rollout time; cheap, but only as accurate as naming conventions and doesn't reflect what's actually called.
- **Service mesh telemetry** (Istio + Kiali) — sidecar-reported call graph; accurate but requires a full mesh install.
- **ML/causal graphs** — Microscope's own causality-graph phase and GNN-based approaches infer edges from statistical correlation of golden-signal time series rather than explicit calls; built for root-cause *ranking*, heavier than this use case needs.

**Chosen approach: trace-based, via Jaeger's own dependency API.** Jaeger (already deployed for tracing, §6.6) exposes `GET /api/dependencies?endTs&lookback`, returning aggregated `(parent, child, callCount)` edges computed from stored spans — the same trace-based technique the research above uses, with zero additional infrastructure since the OTLP wiring already exists.

**Design (`internal/operator/dependency/jaeger.go`):**
- `JaegerDependencyGraph` polls this endpoint on an interval (default 2 min, `DEPENDENCY_GRAPH_REFRESH_SECONDS`) over a lookback window (default 30 min, `DEPENDENCY_GRAPH_LOOKBACK_MINUTES`) and atomically swaps in a fresh graph — no operator restart, no redeploy, a new service or new call path is picked up the refresh cycle after it starts being called.
- **Safety net, not a replacement:** each refresh *merges* observed edges with the hand-declared seed graph rather than replacing it. A known contract relationship must not vanish from the graph just because traffic was quiet in the last lookback window — that would silently disable batch rollback for it during the exact moment (an active incident) it's needed. On Jaeger being unreachable, the previous snapshot (seed-only at cold start) is kept and a warning logged — never an empty graph.
- **Noise filtering:** edges below `DEPENDENCY_GRAPH_MIN_CALL_COUNT` (default 3) are dropped, so a single stray request doesn't wire in a spurious batch-rollback relationship. Self-loops are dropped outright.
- Every refresh diffs against the previous snapshot and logs `edges_added` / `edges_removed` — the graph's evolution is itself part of the audit trail feeding the runbook.
- Set `DEPENDENCY_GRAPH_DYNAMIC_ENABLED=false` to fall back to the static graph only.

**Testing:** 6 unit tests (`internal/operator/dependency/jaeger_test.go`) against a fake Jaeger client — merge-with-seed, new-service-discovered-without-restart (polls until a fabricated `notification-service` edge appears), fallback-when-unreachable, noise-threshold filtering, self-loop rejection, and 20-goroutine concurrent-read-during-refresh under `-race`. All pass.

**Honest limitation:** batch-window membership (§6.3) is still inferred from Helm deploy-time proximity, not an explicit release identifier — the dynamic graph fixes *which services are coupled*, not *which deploys belong to the same release*. A CI/CD pipeline stamping a shared batch/release ID on co-deployed Helm revisions would close that remaining gap.

**Live verification:** pending — the local cluster was down (Docker Desktop not running) when this was implemented. Unit tests confirm correctness in isolation; cluster verification (confirm `edges_added` in operator logs reflecting real order↔inventory traffic, and that batch rollback consults the live graph) is the next step once the cluster is back up.

## 6.8 Pre-existing issue discovered (not fixed, flagged)

Running the full suite with `-race` (previously only run selectively on individual packages) surfaced a **pre-existing data race** in `internal/operator/incident/manager.go`, unrelated to any change in this document: `ProcessAlert` dispatches `go mgr.runRecoveryPipeline(ctx, incident, ...)` and then continues mutating the same `*models.Incident` pointer (`incident.DecisionHistory`, `incident.LastUpdate`) in the calling goroutine while the dispatched goroutine concurrently calls `stateMachine.Transition`, which also writes incident fields. Under concurrent alerts this is undefined behavior — not silently benign — and could corrupt incident state. Not fixed here (out of scope for this session's ask); worth a follow-up: either snapshot/copy the incident before dispatch, or guard the `Incident` struct's mutable fields with a mutex.

## 8. Part 3 — Dependency Batch-Rollback Hardening & Database Rollback Safety (2026-07-25)

A code review of the batch-rollback coordinator (§6.3) and an audit of how
the platform handles persistent data across a rollback, prompted by the
question "if a callee fails and rolls back, do we also roll back callers
still on the new contract — and what happens to any database involved?"

### F9 — Batch peer rollback claimed the shared execution gate before knowing it would execute  **[FIXED]**

`Coordinator.considerPeer` claimed `CooldownManager.TryStartExecution` —
the same per-service gate the alert-driven pipeline uses to prevent
oscillation (F3) — *before* calling `GetLastHealthyRevision`. If that lookup
failed (e.g. every historical revision for the peer was `Superseded`/`Failed`,
none `Deployed`), the gate was still consumed for the full `gateWindow` (5 min
default) even though no rollback was ever attempted. A legitimate alert
firing for that same peer moments later would find its execution gate already
claimed and be suppressed into `Waiting` — the exact false-suppression
scenario F5 already flagged, but reachable here without any real rollback
having happened at all.

**Fix:** the target revision is now resolved *before* the gate is claimed;
the gate is only consumed once a rollback is actually about to be executed.
Regression test: `TestPeerGateNotClaimedWhenNoHealthyTarget`.

### F10 — Batch peer rollbacks were reported successful without any health check  **[FIXED]**

Unlike the primary rollback (which gained SLO-aware verification in §6.4
specifically because pod health alone is a false signal — F1's root cause),
`considerPeer` returned `RolledBack: true` the instant the Helm SDK's
`Rollback` call returned, with no verification step at all. A peer rolled
back onto a revision that never comes up healthy (image pull failure, a
second latent fault, anything) was indistinguishable from a fully healthy
recovery in the incident timeline.

**Fix:** `Coordinator.WithVerifier` wires in the same `VerificationEngine`
the primary path uses (structurally, via a local `PodVerifier` interface —
no import coupling between the `dependency` and `verification` packages).
`PeerResult` now carries a `Verified` field separate from `RolledBack`; an
unverified peer rollback is logged at `ERROR` and recorded with
`VerificationResult: "Unverified"` rather than silently reported as a
success. Regression test: `TestPeerRolledBackButUnverified`.

### F11 — Batch peer rollbacks were invisible to MTTR dashboards and runbooks  **[FIXED]**

Peer rollback outcomes were only ever written to the incident's text
timeline. `rollback_history` (the table `ListRollbacks`, the MTTR dashboards,
and the `GET /runbooks` endpoint all read) never got an entry for a batch
peer, and no Prometheus metric distinguished a peer rollback from a primary
one. A dependency-driven recovery of `inventory-service` while `order-service`
was the alert-triggered primary would show up nowhere except a log line.

**Fix:** every `RolledBack` peer result is now recorded via
`rollbackStore.RecordRollback` (same table, same incident ID as the primary)
and counted in a new `operator_dependency_batch_rollbacks_total{service,
namespace, result}` metric (`result` ∈ `verified` / `unverified`).

### Database rollback safety (the "what about the data" question)

Investigated directly rather than assumed. Full writeup:
[`docs/data-safety-and-migrations.md`](../data-safety-and-migrations.md).
Summary:

- **The three business microservices have no persistent datastore at all** —
  `services/*/repository/repository.go` are in-memory maps. A Helm rollback
  cannot lose or corrupt their data because a pod restart already discards it
  regardless of which revision is live. No migration story is needed for them
  today.
- **The operator's own Postgres incident-audit store is the platform's one
  real, persistent schema.** Its `migrate()` used to be an unconditional list
  of `CREATE TABLE IF NOT EXISTS` statements with no version tracking. It is
  now a versioned, forward-only migration runner
  (`internal/operator/storage/migrations.go`) with an expand/contract
  discipline and a startup **compatibility guard**: the operator refuses to
  boot if the database carries a migration marked `Breaking` whose version
  exceeds what the running binary understands — the exact fingerprint of the
  operator's own Helm release having been rolled back past a schema change
  its code no longer matches. This turns a silent data-corruption/crash risk
  into a loud, actionable startup failure, and mirrors F1's core lesson
  (verify, don't assume success) one layer down, at the schema level instead
  of the SLO level.

---

## 9. Conclusions

1. **The SLO → Prometheus → Alertmanager → operator → Helm-rollback loop works end-to-end** on a real cluster: every SLO-detected failure was automatically rolled back, with MTTR 137–196 s, dominated ~95 % by detection windows (rule `for:` durations + rate windows + Alertmanager group wait), not by the operator (~1 s decide-and-execute).
2. **Chaos testing the healer matters as much as chaos testing the services.** The unit-tested, demo-validated operator harbored three interacting defects (F1–F3) that only surfaced under sustained real alert traffic with multi-replica services — the exact conditions integration demos tend to skip.
3. **Reported success must be verified against the SLO, not just pod health.** The thrashing operator *reported* every oscillating rollback as "Recovered/Healthy" while the cluster was actually serving injected faults. Extending post-rollback verification to query Prometheus for the violated SLO metric (the operator already has a Prometheus client) is the highest-value follow-up.
4. **Kubernetes-level containment (`maxUnavailable: 0`) and SLO-level healing are complementary**, and the seam between them (F4: contained-but-stuck rollouts) needs its own critical alert to be covered.
5. **The execution gate converts oscillation into damped convergence.** Post-fix, each recovery window permits exactly one rollback, and the config-skip guarantees each rollback changes the configuration — so the system steps between configs at most once per cooldown window instead of thrashing, with alert resolution naturally terminating the walk on a good config. The remaining exposure (F5) is the latency of that walk, bounded by Alertmanager's re-notification cadence.
