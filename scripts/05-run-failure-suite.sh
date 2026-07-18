#!/bin/bash
# scripts/05-run-failure-suite.sh
#
# Automated failure-simulation suite. Runs every failure scenario end-to-end
# against the live cluster, measures MTTR (injection → operator-initiated
# Helm rollback), verifies the cluster returns to a clean configuration, and
# writes a markdown report to docs/experiments/.
#
# Prerequisites:
#   - Cluster deployed (scripts/01 + 02) and traffic flowing (scripts/03 or
#     the in-cluster traffic-gen pod), otherwise rate()-based alerts see no
#     samples and nothing fires.
#
# Usage:
#   ./scripts/05-run-failure-suite.sh                # run all scenarios
#   ./scripts/05-run-failure-suite.sh s1 s5          # run selected scenarios
#
# Automation: this script is non-interactive, so it can run from cron or CI:
#   0 3 * * 1  cd ~/project && ./scripts/05-run-failure-suite.sh
#
# Scenario design (docs/experiments/live-failure-simulation-findings.md):
#   every fault is injected by `helm upgrade --set …` (reproducible), detected
#   by a specific PrometheusRule alert, and recovered by the rollback-operator.

set -u

NS=default
REPORT="docs/experiments/suite-$(date +%Y%m%d-%H%M%S).md"
SETTLE_SECONDS=180          # drain alert windows between scenarios
POLL_SECONDS=5

# scenario id → "service|watch_ticks|description|helm --set args"
scenario_def() {
    case "$1" in
        s1) echo "order-service|90|High 5xx error rate (50%)|--set errorRate=0.5" ;;
        s2) echo "inventory-service|120|P99 latency spike (3s/request)|--set latencyMs=3000" ;;
        s3) echo "payment-service|90|Error burst vs strict 1% SLO (30%)|--set errorRate=0.3" ;;
        s4) echo "order-service|120|Combined 20% errors + 1s latency|--set errorRate=0.2 --set latencyMs=1000" ;;
        s5) echo "order-service|140|CrashLoopBackOff via invalid config (stuck rollout)|--set errorRate=5.0" ;;
        *)  return 1 ;;
    esac
}

ALL_SCENARIOS="s1 s2 s3 s4 s5"
SCENARIOS="${*:-$ALL_SCENARIOS}"

ts() { date +%H:%M:%S; }

current_rev() {
    helm history "$1" -n $NS --max 1 -o json 2>/dev/null | \
        python3 -c "import sys,json; r=json.load(sys.stdin)[-1]; print(r['revision'])" 2>/dev/null
}

last_desc() {
    helm history "$1" -n $NS --max 1 -o json 2>/dev/null | \
        python3 -c "import sys,json; r=json.load(sys.stdin)[-1]; print(r.get('description',''))" 2>/dev/null
}

restore_clean() {
    helm upgrade "$1" "./helm/$1" -n $NS \
        --set image.pullPolicy=Never --set image.tag=v1 \
        --set errorRate=0.0 --set latencyMs=0 >/dev/null 2>&1
    kubectl rollout status "deploy/$1" -n $NS --timeout=180s >/dev/null 2>&1
}

echo "# Automated Failure-Simulation Suite — $(date '+%Y-%m-%d %H:%M')" > "$REPORT"
echo "" >> "$REPORT"
echo "| Scenario | Service | Injection | Rollback | MTTR | Result |" >> "$REPORT"
echo "|---|---|---|---|---|---|" >> "$REPORT"

overall_pass=1

for sid in $SCENARIOS; do
    def=$(scenario_def "$sid") || { echo "unknown scenario: $sid"; continue; }
    svc=$(echo "$def"   | cut -d'|' -f1)
    ticks=$(echo "$def" | cut -d'|' -f2)
    desc=$(echo "$def"  | cut -d'|' -f3)
    args=$(echo "$def"  | cut -d'|' -f4)

    echo ""
    echo "━━━ [$sid] $desc → $svc ━━━"

    base=$(current_rev "$svc")
    if [[ -z "$base" ]]; then
        echo "  SKIP: no helm release found for $svc"
        echo "| $sid: $desc | $svc | — | — | — | SKIP (no release) |" >> "$REPORT"
        continue
    fi

    t0=$(date +%s)
    # shellcheck disable=SC2086
    helm upgrade "$svc" "./helm/$svc" -n $NS \
        --set image.pullPolicy=Never --set image.tag=v1 $args >/dev/null 2>&1
    inj=$((base + 1))
    echo "  [$(ts)] injected (revision $inj), watching up to $((ticks * POLL_SECONDS))s for auto-rollback…"

    result="FAIL"
    mttr="—"
    for i in $(seq 1 "$ticks"); do
        rev=$(current_rev "$svc")
        if [[ -n "$rev" && "$rev" -gt "$inj" ]]; then
            t1=$(date +%s)
            mttr="$((t1 - t0))s"
            result="PASS"
            echo "  [$(ts)] auto-rollback detected: revision $rev ($(last_desc "$svc")) — MTTR $mttr"
            break
        fi
        sleep $POLL_SECONDS
    done

    if [[ "$result" == "FAIL" ]]; then
        overall_pass=0
        echo "  [$(ts)] NO auto-rollback within window — restoring manually"
        restore_clean "$svc"
    else
        # Wait for the rolled-back pods, then confirm chaos values are clean;
        # if the rollback landed on a dirty revision, that's a failure too.
        kubectl rollout status "deploy/$svc" -n $NS --timeout=180s >/dev/null 2>&1
        dirty=$(helm get values "$svc" -n $NS 2>/dev/null | grep -E "errorRate|latencyMs" | grep -vE '"?0(\.0)?"?$' || true)
        if [[ -n "$dirty" ]]; then
            result="FAIL (rolled back to dirty config)"
            overall_pass=0
            echo "  [$(ts)] rollback target still carries chaos values — restoring manually"
            restore_clean "$svc"
        fi
    fi

    echo "| $sid: $desc | $svc | $(ts) rev $inj | $(last_desc "$svc") | $mttr | $result |" >> "$REPORT"

    echo "  settling ${SETTLE_SECONDS}s (alert windows drain)…"
    sleep $SETTLE_SECONDS
done

{
    echo ""
    echo "## Post-suite state"
    echo ""
    echo '```'
    kubectl get pods -n $NS
    echo '```'
    echo ""
    echo "Suite result: $([ $overall_pass -eq 1 ] && echo '**ALL PASS**' || echo '**FAILURES PRESENT** — see table')"
} >> "$REPORT"

echo ""
echo "━━━ Suite complete. Report: $REPORT ━━━"
[ $overall_pass -eq 1 ]
