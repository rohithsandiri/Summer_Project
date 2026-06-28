#!/bin/bash
# scripts/03-generate-traffic.sh
# Continuously sends traffic to all services so Prometheus has metrics to evaluate.
# Run this in a dedicated terminal — let it keep running.

set -e

echo "Starting traffic generator..."
echo "Press Ctrl+C to stop"
echo ""

# Get service URLs
ORDER_URL=$(minikube service order-service --url 2>/dev/null || echo "http://localhost:8080")
INVENTORY_URL=$(minikube service inventory-service --url 2>/dev/null || echo "http://localhost:8081")
PAYMENT_URL=$(minikube service payment-service --url 2>/dev/null || echo "http://localhost:8082")

echo "Sending traffic to:"
echo "  Orders:    $ORDER_URL"
echo "  Inventory: $INVENTORY_URL"
echo "  Payment:   $PAYMENT_URL"
echo ""

# Continuous loop — sends ~10 requests/second to each service
i=0
while true; do
    i=$((i + 1))

    # Order service endpoints
    curl -sf "${ORDER_URL}/api/orders" > /dev/null 2>&1 &
    curl -sf "${ORDER_URL}/api/orders/create" > /dev/null 2>&1 &
    curl -sf "${ORDER_URL}/api/orders/status" > /dev/null 2>&1 &

    # Inventory service endpoints
    curl -sf "${INVENTORY_URL}/api/inventory" > /dev/null 2>&1 &
    curl -sf "${INVENTORY_URL}/api/inventory/check" > /dev/null 2>&1 &

    # Payment service endpoints
    curl -sf "${PAYMENT_URL}/api/payment/process" > /dev/null 2>&1 &
    curl -sf "${PAYMENT_URL}/api/payment/status" > /dev/null 2>&1 &

    # Print status every 10 iterations
    if (( i % 10 == 0 )); then
        echo "[$(date +%H:%M:%S)] Sent $((i * 7)) total requests..."
    fi

    sleep 0.5
done
