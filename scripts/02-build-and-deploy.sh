#!/bin/bash
# scripts/02-build-and-deploy.sh
# Builds Docker images, loads them into minikube, deploys via Helm.
# Run this AFTER 01-setup-cluster.sh

set -e
set -u

REGISTRY="rohithsandiri"
VERSION="v1"

echo "╔══════════════════════════════════════════════════════╗"
echo "║   P11: Build Images + Deploy Services                ║"
echo "╚══════════════════════════════════════════════════════╝"

# ── Step 1: Point Docker to minikube's Docker daemon ─────────────────────
# This means images we build go DIRECTLY into minikube — no push to registry needed
eval $(minikube docker-env)
echo "✓ Docker pointed at minikube daemon"

# ── Step 2: Build all service images ──────────────────────────────────────
echo ""
echo "▶ Building Docker images..."

# Order service
echo "  Building order-service:v1..."
docker build -t ${REGISTRY}/order-service:v1 \
    -f services/order-service/Dockerfile .
echo "  ✓ order-service:v1"

# Inventory service
echo "  Building inventory-service:v1..."
docker build -t ${REGISTRY}/inventory-service:v1 \
    -f services/inventory-service/Dockerfile .
echo "  ✓ inventory-service:v1"

# Payment service
echo "  Building payment-service:v1..."
docker build -t ${REGISTRY}/payment-service:v1 \
    -f services/payment-service/Dockerfile .
echo "  ✓ payment-service:v1"

# Rollback operator
echo "  Building rollback-operator:v1..."
docker build -t ${REGISTRY}/rollback-operator:v1 \
    -f operator/Dockerfile .
echo "  ✓ rollback-operator:v1"

echo ""
echo "▶ Docker images in minikube:"
docker images | grep ${REGISTRY}

# ── Step 3: Apply RBAC for operator ───────────────────────────────────────
echo ""
echo "▶ Applying operator RBAC..."
kubectl apply -f k8s/operator-rbac.yaml
echo "✓ RBAC applied"

# ── Step 4: Deploy microservices via Helm ────────────────────────────────
echo ""
echo "▶ Deploying microservices..."

helm upgrade --install order-service ./helm/order-service \
    --namespace default \
    --set image.repository=${REGISTRY}/order-service \
    --set image.tag=v1 \
    --set image.pullPolicy=Never \
    --wait

helm upgrade --install inventory-service ./helm/inventory-service \
    --namespace default \
    --set image.repository=${REGISTRY}/inventory-service \
    --set image.tag=v1 \
    --set image.pullPolicy=Never \
    --wait

helm upgrade --install payment-service ./helm/payment-service \
    --namespace default \
    --set image.repository=${REGISTRY}/payment-service \
    --set image.tag=v1 \
    --set image.pullPolicy=Never \
    --wait

echo "✓ Microservices deployed"

# ── Step 5: Deploy rollback operator ──────────────────────────────────────
echo ""
echo "▶ Deploying rollback operator..."
helm upgrade --install rollback-operator ./helm/rollback-operator \
    --namespace default \
    --set image.repository=${REGISTRY}/rollback-operator \
    --set image.tag=v1 \
    --set image.pullPolicy=Never \
    --wait
echo "✓ Rollback operator deployed"

# ── Step 6: Apply SLO alerting rules and Alertmanager config ──────────────
echo ""
echo "▶ Applying Prometheus rules and Alertmanager config..."
kubectl apply -f k8s/prometheus-rules/slo-alerting-rules.yaml
kubectl apply -f k8s/alertmanager/alertmanager-config.yaml
echo "✓ Rules and config applied"

# ── Step 7: Verify everything is running ──────────────────────────────────
echo ""
echo "▶ Pod status:"
kubectl get pods -n default
kubectl get pods -n monitoring

echo ""
echo "▶ Helm releases:"
helm list -A

echo ""
echo "══════════════════════════════════════════════════════"
echo "✓ All services deployed! Next: run 03-generate-traffic.sh"
echo "══════════════════════════════════════════════════════"
