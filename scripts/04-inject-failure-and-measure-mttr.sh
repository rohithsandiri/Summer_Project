#!/bin/bash
# scripts/04-inject-failure-and-measure-mttr.sh
#
# This is your RESEARCH EXPERIMENT script.
# For each scenario:
#   1. Record the baseline time
#   2. Inject failure via helm upgrade
#   3. Wait for the operator to auto-rollback
#   4. Record recovery time = MTTR
#
# Run this while 03-generate-traffic.sh is running in another terminal.

set -e

REGISTRY="rohithsandiri"

# ── Helper functions ───────────────────────────────────────────────────────

inject_failure() {
    local service=$1
    local description=$2
    shift 2
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "INJECTING: $description → $service"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    helm upgrade ${service} ./helm/${service} \
        --namespace default \
        --set image.pullPolicy=Never \
        "$@"
    echo "Failure injected at $(date +%H:%M:%S)"
    echo "Watching for auto-rollback..."
}

wait_for_recovery() {
    local service=$1
    local start_time=$2

    # Watch helm history — operator will add a new revision when it rollbacks
    while true; do
        latest_revision=$(helm history ${service} --namespace default --max 1 -o json \
            | python3 -c "import sys,json; h=json.load(sys.stdin); print(h[-1]['status'])" 2>/dev/null || echo "unknown")

        if [[ "$latest_revision" == "deployed" ]]; then
            end_time=$(date +%s)
            mttr=$((end_time - start_time))
            echo ""
            echo "✓ RECOVERY CONFIRMED at $(date +%H:%M:%S)"
            echo "  MTTR = ${mttr} seconds"
            helm history ${service} --namespace default
            return $mttr
        fi

        echo -n "."
        sleep 5
    done
}

# ── Scenario 1: High Error Rate ────────────────────────────────────────────
run_scenario_1() {
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║  SCENARIO 1: High Error Rate (50%)               ║"
    echo "╚══════════════════════════════════════════════════╝"

    START=$(date +%s)
    inject_failure "order-service" "50% error rate injection" \
        --set errorRate=0.5

    wait_for_recovery "order-service" $START
    echo "Scenario 1 complete. MTTR logged."
    sleep 30
}

# ── Scenario 2: High P99 Latency ──────────────────────────────────────────
run_scenario_2() {
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║  SCENARIO 2: High P99 Latency (3 second delay)  ║"
    echo "╚══════════════════════════════════════════════════╝"

    START=$(date +%s)
    inject_failure "inventory-service" "3 second latency injection" \
        --set latencyMs=3000

    wait_for_recovery "inventory-service" $START
    echo "Scenario 2 complete."
    sleep 30
}

# ── Scenario 3: Payment Service Outage (high error rate, strict SLO) ──────
run_scenario_3() {
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║  SCENARIO 3: Payment gateway error rate 30%     ║"
    echo "╚══════════════════════════════════════════════════╝"

    START=$(date +%s)
    inject_failure "payment-service" "30% payment errors" \
        --set errorRate=0.3

    wait_for_recovery "payment-service" $START
    echo "Scenario 3 complete."
    sleep 30
}

# ── Scenario 4: Combined: error + latency ─────────────────────────────────
run_scenario_4() {
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║  SCENARIO 4: Combined error + latency failure    ║"
    echo "╚══════════════════════════════════════════════════╝"

    START=$(date +%s)
    inject_failure "order-service" "20% errors + 1s latency" \
        --set errorRate=0.2 \
        --set latencyMs=1000

    wait_for_recovery "order-service" $START
    echo "Scenario 4 complete."
    sleep 30
}

# ── Run all scenarios ──────────────────────────────────────────────────────
echo "Starting MTTR benchmarking experiments..."
echo "Make sure 03-generate-traffic.sh is running in another terminal!"
echo ""
read -p "Press ENTER to begin..."

run_scenario_1
run_scenario_2
run_scenario_3
run_scenario_4

echo ""
echo "╔══════════════════════════════════════════════════╗"
echo "║  All scenarios complete! Collecting MTTR data...  ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""
echo "View MTTR metrics in Prometheus:"
echo "  http://localhost:9090"
echo "  Query: operator_mttr_seconds"
echo ""
echo "Helm rollback history:"
helm history order-service --namespace default
helm history inventory-service --namespace default
helm history payment-service --namespace default
