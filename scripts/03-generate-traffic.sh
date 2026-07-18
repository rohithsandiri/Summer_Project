#!/bin/bash
# scripts/03-generate-traffic.sh
# Continuously sends order-creation traffic to order-service (which in turn
# calls inventory-service and payment-service internally), so Prometheus has
# metrics to evaluate. Run this in a dedicated terminal — let it keep running.

set -e

NAMESPACE="default"
LOCAL_PORT="18080"

echo "Starting traffic generator..."
echo "Press Ctrl+C to stop"
echo ""

# order-service is ClusterIP-only inside the cluster, so port-forward it to
# the host rather than relying on `minikube service` (which needs NodePort).
kubectl port-forward -n "$NAMESPACE" svc/order-service "${LOCAL_PORT}:8080" >/tmp/order-service-port-forward.log 2>&1 &
PF_PID=$!
trap 'echo ""; echo "Stopping traffic generator..."; kill $PF_PID 2>/dev/null; exit 0' INT TERM

# Wait for the port-forward to become ready
for i in $(seq 1 20); do
    if curl -sf "http://localhost:${LOCAL_PORT}/health" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

ORDER_URL="http://localhost:${LOCAL_PORT}"
echo "Sending traffic to order-service via port-forward: ${ORDER_URL}"
echo ""

SKUS=("SKU-001" "SKU-002" "SKU-003" "SKU-004" "SKU-005")

# Continuous loop — sends ~2 order-creation requests/second
i=0
while true; do
    i=$((i + 1))
    sku=${SKUS[$((i % 5))]}

    curl -sf -X POST "${ORDER_URL}/orders" \
        -H "Content-Type: application/json" \
        -d "{\"customer_id\":\"cust-${i}\",\"currency\":\"USD\",\"items\":[{\"item_id\":\"${sku}\",\"quantity\":1,\"unit_price\":1000}]}" \
        > /dev/null 2>&1 &

    if (( i % 10 == 0 )); then
        echo "[$(date +%H:%M:%S)] Sent $i order requests..."
    fi

    sleep 0.5
done
