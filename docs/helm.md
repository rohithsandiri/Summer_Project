# Helm Charts & Rollback Architecture

This document outlines the Helm packaging standards, templating strategy, deployment configurations, and automated SDK rollback execution pipeline.

---

## 1. Chart Structure

The platform uses individual Helm charts for each component, organized under the `helm/` directory:

```
helm/
├── order-service/        # Order service chart
│   ├── Chart.yaml
│   ├── values.yaml       # Default healthy configs (v1)
│   └── templates/        # K8s Deployment, Service, ServiceMonitor, PriorityClass
├── inventory-service/    # Inventory service chart
├── payment-service/      # Payment service chart
└── rollback-operator/    # Operator control plane chart
```

Each chart is fully self-contained and exposes environment failure parameters (`errorRate`, `latencyMs`) read by the Go binary at startup.

---

## 2. Values File Hierarchy

To support different environments, each microservice includes multiple target values files:

- **`values.yaml`**: Standard defaults for local testing or minikube deployment.
- **`values-dev.yaml`**: Configured with single replicas and minimal memory footprint (`32Mi`) for fast startup.
- **`values-staging.yaml`**: Configured with dual replicas and enabled ServiceMonitors for integration testing.
- **`values-prod.yaml`**: Strict configuration using 3 replicas, hard pod anti-affinities, security context locks, and topology spread constraints.

---

## 3. Deployment Templates Configuration

The workload templates are highly hardened using variables:

```yaml
# Inside deployment.yaml template
spec:
  replicas: {{ .Values.replicaCount }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}
        version: {{ .Values.image.tag | quote }}
        helm-release: {{ .Release.Name }}   # Tracked by Operator to trigger rollbacks
```

---

## 4. The Rollback Mechanism

The operator utilizes the **Helm Go SDK** to perform rollbacks programmatically rather than calling shell commands.

### The Rollback Pipeline:
1. **Webhook Detection**: Alertmanager delivers a critical alert webhook to the operator.
2. **Retrieve Release Name**: The operator extracts the `helm-release` label value from the alert context metadata.
3. **Execute SDK Rollback**:
   ```go
   // Go SDK implementation snippet used by the Recovery Executor
   cfg := new(action.Configuration)
   err := cfg.Init(kubeConfig, namespace, "secret", log.Printf)
   rollbackClient := action.NewRollback(cfg)
   rollbackClient.Version = 0 // Rollback to immediately preceding healthy revision (N-1)
   err = rollbackClient.Run(releaseName)
   ```
4. **Target Version (N-1)**: By setting the target version to `0`, Helm automatically resolves the immediate previous release index and restores all associated Deployment, ConfigMap, and Service states.
5. **Rollback History Audit**: The operator writes the outcome, duration, and target version to Postgres database tables for reliability analytics.
