#!/bin/bash
# scripts/01-setup-cluster.sh
# Run this FIRST. Sets up minikube, installs kube-prometheus-stack.
# Prerequisites: minikube, kubectl, helm already installed.

set -e   # exit on error
set -u   # treat unset vars as errors

echo "╔══════════════════════════════════════════════════════╗"
echo "║   P11: Self-Healing K8s — Cluster Setup Script       ║"
echo "╚══════════════════════════════════════════════════════╝"

# ── Step 1: Start minikube ─────────────────────────────────────────────────
echo ""
echo "▶ Step 1: Starting minikube..."
minikube start \
    --memory=6144 \
    --cpus=4 \
    --disk-size=30g \
    --driver=docker

# Enable metrics-server for resource metrics
minikube addons enable metrics-server

echo "✓ minikube started"
kubectl get nodes

# ── Step 2: Install kube-prometheus-stack ─────────────────────────────────
# This single Helm chart installs: Prometheus + Alertmanager + Grafana + node-exporter
echo ""
echo "▶ Step 2: Installing kube-prometheus-stack (Prometheus + Alertmanager + Grafana)..."

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
    --namespace monitoring \
    --set grafana.adminPassword=admin123 \
    --set grafana.service.type=NodePort \
    --set prometheus.service.type=NodePort \
    --set alertmanager.service.type=NodePort \
    --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
    --set prometheus.prometheusSpec.ruleSelectorNilUsesHelmValues=false \
    --wait \
    --timeout=10m

echo "✓ kube-prometheus-stack installed"

# ── Step 3: Verify Prometheus is running ──────────────────────────────────
echo ""
echo "▶ Step 3: Verifying Prometheus stack..."
kubectl get pods -n monitoring

echo ""
echo "══════════════════════════════════════════════════════"
echo "Access URLs (run these in separate terminals):"
echo ""
echo "  Prometheus UI:"
echo "  kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-prometheus 9090:9090"
echo "  → http://localhost:9090"
echo ""
echo "  Grafana UI:"
echo "  kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80"
echo "  → http://localhost:3000  (admin / admin123)"
echo ""
echo "  Alertmanager UI:"
echo "  kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-alertmanager 9093:9093"
echo "  → http://localhost:9093"
echo "══════════════════════════════════════════════════════"
