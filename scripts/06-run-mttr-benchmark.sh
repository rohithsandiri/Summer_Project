#!/bin/bash
# scripts/06-run-mttr-benchmark.sh
#
# Human-baseline vs. automated-rollback MTTR benchmark.
#
# Extends scripts/05-run-failure-suite.sh's single-shot scenario suite into a
# repeated, two-arm comparison suitable for statistical analysis:
#   automated arm: the rollback-operator recovers on its own (scripts/05's path).
#   manual arm:    the operator is scaled to zero; this script plays the human
#                  — it detects the first instant the scenario's alert reaches
#                  FIRING in Prometheus (the same signal a dashboard-watching
#                  human would see), waits a randomly-sampled reaction time,
#                  then runs `helm rollback` itself.
#
# Why a distribution, not a fixed reaction time: a single fixed constant
# (e.g. "45s") models a point estimate, not human variability, and cannot be
# grounded against a cited range the way a sampled distribution can. Each
# manual rep draws its own reaction time uniformly from
# [REACTION_MIN_SECONDS, REACTION_MAX_SECONDS] (default 30-60s, i.e. someone
# already at a screen who knows exactly which release to roll back) — still a
# simplification (see the paper's Threats to Validity), but no longer a
# single point.
#
# Isolation: every rep, in both arms, is gated on Prometheus reporting ZERO
# firing alerts for the target service before injection — a rep that starts
# while the previous rep's alert is still resolving isn't measuring detection
# at all (see docs/experiments/live-failure-simulation-findings.md, "stale
# alert contamination").
#
# Output: append-only CSVs, one row per rep, suitable for scripts/analyze_mttr.py:
#   scenarios/results-automated.csv
#   scenarios/results-manual.csv
#
# Usage:
#   ./scripts/06-run-mttr-benchmark.sh                 # all scenarios, 15 reps/arm (default)
#   ./scripts/06-run-mttr-benchmark.sh --reps 30 s1 s3  # 30 reps, only s1 and s3
#
# Prerequisites: same as scripts/05, PLUS a port-forward to Prometheus:
#   kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-prometheus 9090:9090

set -u

NS=default
OPERATOR_DEPLOY=rollback-operator
PROM_URL="${PROMETHEUS_URL:-http://localhost:9090}"
REACTION_MIN_SECONDS="${REACTION_MIN_SECONDS:-30}"
REACTION_MAX_SECONDS="${REACTION_MAX_SECONDS:-60}"
POLL_SECONDS=5
MAX_WAIT_SECONDS=600         # per-rep hard timeout for either arm
STALE_CHECK_TIMEOUT=120      # how long to wait for pre-existing alerts to clear before giving up on a rep

RESULTS_DIR="scenarios"
AUTO_CSV="$RESULTS_DIR/results-automated.csv"
MANUAL_CSV="$RESULTS_DIR/results-manual.csv"
mkdir -p "$RESULTS_DIR"
[[ -f "$AUTO_CSV" ]]   || echo "scenario,rep,t0_unix,t1_unix,mttr_seconds,detection_seconds,result,notes" > "$AUTO_CSV"
[[ -f "$MANUAL_CSV" ]] || echo "scenario,rep,t0_unix,detected_unix,reaction_seconds,t1_unix,mttr_seconds,detection_seconds,result,notes" > "$MANUAL_CSV"

# scenario id → "service|alertname|description|helm --set args"
scenario_def() {
    case "$1" in
        s1) echo "order-service|OrderServiceHighErrorRate|High 5xx error rate (50%)|--set errorRate=0.5" ;;
        s2) echo "inventory-service|InventoryServiceHighLatency|P99 latency spike (3s/request)|--set latencyMs=3000" ;;
        s3) echo "payment-service|PaymentServiceHighErrorRate|Error burst vs strict 1% SLO (30%)|--set errorRate=0.3" ;;
        s5) echo "order-service|OrderServiceStuckRollout|CrashLoopBackOff via invalid config|--set errorRate=5.0" ;;
        *)  return 1 ;;
    esac
}
ALL_SCENARIOS="s1 s2 s3 s5"   # s4 (combined fault) excluded: see paper Section on internal validity — cooldown contamination made it uninterpretable in the pilot run.

REPS=15
SCENARIOS=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --reps) REPS="$2"; shift 2 ;;
        *) SCENARIOS="$SCENARIOS $1"; shift ;;
    esac
done
SCENARIOS="${SCENARIOS:-$ALL_SCENARIOS}"

ts() { date +%H:%M:%S; }
now() { date +%s; }

current_rev() {
    helm history "$1" -n $NS --max 1 -o json 2>/dev/null | \
        python3 -c "import sys,json; r=json.load(sys.stdin)[-1]; print(r['revision'])" 2>/dev/null
}

restore_clean() {
    helm upgrade "$1" "./helm/$1" -n $NS \
        --set image.pullPolicy=Never --set image.tag=v1 \
        --set errorRate=0.0 --set latencyMs=0 >/dev/null 2>&1
    kubectl rollout status "deploy/$1" -n $NS --timeout=180s >/dev/null 2>&1
}

# firing_count SVC_ALERTNAME → number of currently-firing instances of that alert
firing_count() {
    curl -s --max-time 5 "$PROM_URL/api/v1/query" \
        --data-urlencode "query=count(ALERTS{alertname=\"$1\",alertstate=\"firing\"}) or vector(0)" \
        2>/dev/null | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(int(float(d['data']['result'][0]['value'][1])))
except Exception:
    print(0)
" 2>/dev/null
}

wait_for_quiet() {
    # Gate a rep's start on zero currently-firing instances of this scenario's
    # alert — prevents a rep from starting mid-resolution of the previous one.
    local alertname="$1" waited=0
    while [[ "$(firing_count "$alertname")" -gt 0 ]]; do
        if [[ $waited -ge $STALE_CHECK_TIMEOUT ]]; then
            echo "  WARN: $alertname still firing after ${STALE_CHECK_TIMEOUT}s wait — proceeding anyway, rep may be contaminated"
            return 1
        fi
        sleep $POLL_SECONDS
        waited=$((waited + POLL_SECONDS))
    done
    return 0
}

wait_for_firing() {
    # Poll until $1's alert reaches FIRING; echo the unix time it did, or
    # nothing on timeout.
    local alertname="$1" deadline=$(( $(now) + MAX_WAIT_SECONDS ))
    while [[ $(now) -lt $deadline ]]; do
        if [[ "$(firing_count "$alertname")" -gt 0 ]]; then
            now; return 0
        fi
        sleep $POLL_SECONDS
    done
    return 1
}

wait_for_recovered() {
    # Poll until the target revision is live AND healthy (rollout complete)
    # AND the alert has cleared in Prometheus — the same dual criterion
    # (pod health + SLO-alert-cleared) the operator's own verification uses.
    local svc="$1" alertname="$2" base_rev="$3" deadline=$(( $(now) + MAX_WAIT_SECONDS ))
    while [[ $(now) -lt $deadline ]]; do
        rev=$(current_rev "$svc")
        if [[ -n "$rev" && "$rev" -gt "$base_rev" ]]; then
            if kubectl rollout status "deploy/$svc" -n $NS --timeout=5s >/dev/null 2>&1; then
                if [[ "$(firing_count "$alertname")" -eq 0 ]]; then
                    now; return 0
                fi
            fi
        fi
        sleep $POLL_SECONDS
    done
    return 1
}

run_automated_rep() {
    local sid="$1" svc="$2" alertname="$3" args="$4" rep="$5"
    kubectl scale deploy/$OPERATOR_DEPLOY -n $NS --replicas=1 >/dev/null 2>&1
    kubectl rollout status "deploy/$OPERATOR_DEPLOY" -n $NS --timeout=60s >/dev/null 2>&1

    wait_for_quiet "$alertname"
    base=$(current_rev "$svc")
    t0=$(now)
    # shellcheck disable=SC2086
    helm upgrade "$svc" "./helm/$svc" -n $NS --set image.pullPolicy=Never --set image.tag=v1 $args >/dev/null 2>&1
    echo "  [$(ts)] [auto  rep $rep] injected, waiting for the operator…"

    if t1=$(wait_for_recovered "$svc" "$alertname" "$base"); then
        mttr=$((t1 - t0))
        echo "$sid,$rep,$t0,$t1,$mttr,,PASS," >> "$AUTO_CSV"
        echo "  [$(ts)] [auto  rep $rep] recovered — MTTR ${mttr}s"
    else
        echo "$sid,$rep,$t0,,,,TIMEOUT," >> "$AUTO_CSV"
        echo "  [$(ts)] [auto  rep $rep] TIMEOUT after ${MAX_WAIT_SECONDS}s — restoring"
        restore_clean "$svc"
    fi
    sleep 30
}

run_manual_rep() {
    local sid="$1" svc="$2" alertname="$3" args="$4" rep="$5"
    kubectl scale deploy/$OPERATOR_DEPLOY -n $NS --replicas=0 >/dev/null 2>&1
    sleep 3

    wait_for_quiet "$alertname"
    base=$(current_rev "$svc")
    t0=$(now)
    # shellcheck disable=SC2086
    helm upgrade "$svc" "./helm/$svc" -n $NS --set image.pullPolicy=Never --set image.tag=v1 $args >/dev/null 2>&1
    echo "  [$(ts)] [manual rep $rep] injected, watching Prometheus for FIRING…"

    if detected=$(wait_for_firing "$alertname"); then
        reaction=$(( RANDOM % (REACTION_MAX_SECONDS - REACTION_MIN_SECONDS + 1) + REACTION_MIN_SECONDS ))
        detect_secs=$((detected - t0))
        echo "  [$(ts)] [manual rep $rep] detected after ${detect_secs}s, simulating ${reaction}s human reaction…"
        sleep "$reaction"
        helm rollback "$svc" 0 -n $NS >/dev/null 2>&1   # 0 = previous revision, same semantics the operator uses

        if t1=$(wait_for_recovered "$svc" "$alertname" "$base"); then
            mttr=$((t1 - t0))
            echo "$sid,$rep,$t0,$detected,$reaction,$t1,$mttr,$detect_secs,PASS," >> "$MANUAL_CSV"
            echo "  [$(ts)] [manual rep $rep] recovered — MTTR ${mttr}s (detection ${detect_secs}s + reaction ${reaction}s + execution $((mttr - detect_secs - reaction))s)"
        else
            echo "$sid,$rep,$t0,$detected,$reaction,,,$detect_secs,TIMEOUT," >> "$MANUAL_CSV"
            echo "  [$(ts)] [manual rep $rep] TIMEOUT waiting for recovery after manual rollback — restoring"
            restore_clean "$svc"
        fi
    else
        echo "$sid,$rep,$t0,,,,,,TIMEOUT,alert never fired" >> "$MANUAL_CSV"
        echo "  [$(ts)] [manual rep $rep] alert never reached FIRING within ${MAX_WAIT_SECONDS}s — restoring"
        restore_clean "$svc"
    fi
    sleep 30
}

echo "=== MTTR benchmark: $REPS reps/arm, scenarios:$SCENARIOS ==="
echo "=== Manual-arm reaction time ~ Uniform(${REACTION_MIN_SECONDS}s, ${REACTION_MAX_SECONDS}s) ==="

for sid in $SCENARIOS; do
    def=$(scenario_def "$sid") || { echo "unknown scenario: $sid"; continue; }
    svc=$(echo "$def"       | cut -d'|' -f1)
    alertname=$(echo "$def" | cut -d'|' -f2)
    desc=$(echo "$def"      | cut -d'|' -f3)
    args=$(echo "$def"      | cut -d'|' -f4)

    echo ""
    echo "━━━ [$sid] $desc → $svc ($alertname) ━━━"

    for rep in $(seq 1 "$REPS"); do
        run_automated_rep "$sid" "$svc" "$alertname" "$args" "$rep"
    done
    for rep in $(seq 1 "$REPS"); do
        run_manual_rep "$sid" "$svc" "$alertname" "$args" "$rep"
    done
done

kubectl scale deploy/$OPERATOR_DEPLOY -n $NS --replicas=1 >/dev/null 2>&1
echo ""
echo "━━━ Benchmark complete. Results: $AUTO_CSV, $MANUAL_CSV ━━━"
echo "━━━ Analyze with: python3 scripts/analyze_mttr.py $AUTO_CSV $MANUAL_CSV ━━━"
