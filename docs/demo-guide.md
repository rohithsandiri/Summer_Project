# Platform SRE Demonstration & Validation Guide

This document provides a step-by-step walkthrough to demonstrate the self-healing and progressive delivery capabilities of the platform.

---

## Step 1: Environment Setup
1. Make sure docker is running.
2. Initialize the Kubernetes cluster and install the Prometheus stack:
   ```bash
   ./scripts/01-setup-cluster.sh
   ```
3. Build the Docker images and deploy the Helm charts:
   ```bash
   ./scripts/02-build-and-deploy.sh
   ```

---

## Step 2: Open Monitoring Consoles
In separate terminals, port-forward the monitoring services:

```bash
# Prometheus console
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-prometheus 9090:9090

# Alertmanager console
kubectl port-forward -n monitoring svc/monitoring-kube-prometheus-alertmanager 9093:9093

# Grafana Dashboard — retrieve credentials with:
# kubectl get secret -n monitoring monitoring-grafana -o jsonpath="{.data.admin-password}" | base64 -d
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80
```

---

## Step 3: Run Load Generator
To simulate continuous client traffic and build a request rate baseline:
```bash
./scripts/03-generate-traffic.sh
```

---

## Step 4: Inject Failure (Trigger Automated Rollback)
Simulate a buggy deployment of `order-service` by introducing a `50%` failure rate:
```bash
# Update release values directly
helm upgrade order-service ./helm/order-service --set errorRate=0.5
```

### Observation Loop:
1. **Prometheus**: Navigate to `http://localhost:9090` and watch the error rate metric rise.
2. **Alertmanager**: Check `http://localhost:9093`. The `HighErrorRate` alert transitions from `Pending` to `Firing`.
3. **Operator Logs**: Watch the operator logs detect the webhook, parse the target helm release, and call the rollback SDK:
   ```bash
   kubectl logs -f deployment/rollback-operator
   ```
4. **Helm Status**: Check the release history of the order service:
   ```bash
   helm history order-service
   ```
   You should see a new revision created as a rollback of the previous version.
5. **System Recovery**: The error rate immediately drops to zero, and the system restores itself.

---

## Step 5: Test Deployment Guard (Progressive Delivery Block)
While `order-service` is unstable (or during an active SLO breach), try to deploy a new version of `payment-service`:
```bash
kubectl apply -f k8s/progressive/payment-rollout.yaml
```
- Check the Argo Rollout status:
  ```bash
  kubectl argo rollouts get rollout payment-service
  ```
- The operator's **Deployment Guard** detects the active incident/high burn rate and blocks the rollout, keeping the existing stable deployment intact.

---

## Step 6: Cleanup
To clean up all cluster resources and terminate the minikube session:
```bash
minikube delete
```
