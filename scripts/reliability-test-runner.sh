#!/usr/bin/env bash
# scripts/reliability-test-runner.sh
#
# Production Reliability & Soak Testing Suite.
# Fulfills Phase 9 Requirement 9: soak tests, continuous chaos, and memory leak detection.

set -euo pipefail

DURATION_MINUTES=${1:-10}
NAMESPACE="default"

echo "======================================================================"
echo "          STARTING RELIABILITY & SOAK TEST SUITE"
echo "          Duration: $DURATION_MINUTES minutes"
echo "======================================================================"

# 1. Start Background Load Test using k6
echo "[1/4] Launching load generator in background..."
if command -v k6 &>/dev/null; then
    k6 run --duration "${DURATION_MINUTES}m" load-testing/medium_load.js &
    LOAD_PID=$!
    echo "✓ k6 load test started in background (PID: $LOAD_PID)"
else
    echo "⚠ k6 load test utility not installed. Running background curl load simulation..."
    while true; do
        curl -s -o /dev/null -H "X-Trace-ID: soak-trace-$(date +%s)" http://localhost:8000/orders || true
        sleep 0.1
    done &
    LOAD_PID=$!
    echo "✓ Simulated load script started in background (PID: $LOAD_PID)"
fi

# 2. Continuous Chaos Loop (Deletes Pods randomly)
echo "[2/4] Initializing continuous chaos injector loop..."
chaos_loop() {
    local services=("order-service" "payment-service" "inventory-service" "api-gateway")
    while true; do
        sleep_time=$((10 + RANDOM % 20))
        sleep "$sleep_time"
        target=${services[$((RANDOM % 4))]}
        echo "[CHAOS] Evicting random pod for service: $target"
        kubectl delete pods -n "$NAMESPACE" -l "app=$target" --grace-period=0 --force 2>/dev/null || true
    done
}
chaos_loop &
CHAOS_PID=$!
echo "✓ Continuous chaos loop started in background (PID: $CHAOS_PID)"

# 3. Memory Leak Detection Loop
echo "[3/4] Starting real-time memory profiling & leak detector..."
memory_monitor_loop() {
    local services=("order-service" "payment-service" "inventory-service")
    local log_file="./backups/memory_leaks.log"
    mkdir -p "./backups"
    echo "Timestamp, Service, MemoryMB" > "$log_file"
    
    while true; do
        sleep 5
        timestamp=$(date +%Y-%m-%d_%H:%M:%S)
        for svc in "${services[@]}"; do
            # Attempt to query from local metrics /metrics endpoints if reachable
            local port=8080
            if [ "$svc" = "payment-service" ]; then port=8082; fi
            if [ "$svc" = "inventory-service" ]; then port=8081; fi
            
            mem_bytes=$(curl -s "http://localhost:$port/metrics" 2>/dev/null | grep "go_memstats_alloc_bytes" | grep -v "#" | awk '{print $2}' || echo "0")
            if [ "$mem_bytes" != "0" ] && [ -n "$mem_bytes" ]; then
                local mem_mb
                mem_mb=$(echo "$mem_bytes / 1024 / 1024" | bc 2>/dev/null || echo "0")
                echo "$timestamp, $svc, ${mem_mb}MB" >> "$log_file"
                
                # Simple threshold check
                if [ "$mem_mb" -gt 256 ]; then
                    echo "⚠ WARNING: Memory leak suspected on $svc! Usage exceeded 256MB: ${mem_mb}MB"
                fi
            fi
        done
    done
}
memory_monitor_loop &
MONITOR_PID=$!
echo "✓ Memory monitor started in background (PID: $MONITOR_PID)"

# 4. Wait for specified duration
echo "[4/4] Soak test running... waiting for $DURATION_MINUTES minutes."
sleep $((DURATION_MINUTES * 60))

# Cleanup
echo "======================================================================"
echo "          CLEANING UP TEST PROCESSES"
echo "======================================================================"
kill "$LOAD_PID" || true
kill "$CHAOS_PID" || true
kill "$MONITOR_PID" || true

echo "✓ Soak test completed."
echo "Memory log saved to: ./backups/memory_leaks.log"
echo "======================================================================"
