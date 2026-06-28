# Kubernetes Manifests & Deployment Infrastructure

This document details the purpose, structure, and configuration of each Kubernetes resource defined under the `k8s/` directory.

---

## 1. Core Resource Architecture

### 1.1 Deployments
Every service is deployed as a stateless `Deployment` inside the target namespace. Deployments use rolling update strategies to prevent downtime:
- `maxSurge: 1`: Creates at most one additional pod during updates.
- `maxUnavailable: 0`: Guarantees that the baseline number of pods remain running at all times.

### 1.2 Services
Cluster IP services provide routing and load balancing within the cluster. Ports are bound as follows:
- **API Gateway**: Exposes port `8000` internally (mapped to external port through NodePort or LoadBalancer).
- **Order Service**: Exposes port `8080`.
- **Inventory Service**: Exposes port `8081`.
- **Payment Service**: Exposes port `8082`.
- **Rollback Operator**: Exposes port `8090` (for Alertmanager webhooks).

### 1.3 ConfigMaps & Secrets
- **ConfigMaps**: Load environment variables and bootstrap properties, such as DB host, port, and rate limits.
- **Secrets**: Handle credentials (such as DB password) securely.

---

## 2. Advanced Scheduling & Pod Safety

### 2.1 Pod Disruption Budgets (PDB)
PDBs protect service availability during node draining, kernel upgrades, or scheduling changes:
- **`k8s/*/pdb.yaml`**: Standard configuration requires at least 1 healthy replica (`minAvailable: 1`) to be active.

### 2.2 Priority Classes
Ensures critical services are scheduled first:
- `PriorityClass` resources are created for each service with a priority value of `10000` (compared to standard user pod priority `0`).

### 2.3 Topology Spread Constraints & Anti-Affinity
- **Anti-Affinity**: Standard configurations prevent pods of the same microservice from co-locating on the same physical host.
- **Topology Spread Constraints**: Configured with a max skew of `1` across hostnames to guarantee even distribution of replicas.

---

## 3. Network Isolation (NetworkPolicies)
We implement a zero-trust network model inside the cluster using `NetworkPolicy` resources:
- **API Gateway**: Allows all ingress traffic, but is restricted to egressing only to Order, Inventory, and Payment services.
- **Microservices**: Accept ingress traffic *only* from the API Gateway and the Order Service (Saga controller). Direct communication between users and backend microservices is blocked.

---

## 4. Observability Integration

### 4.1 ServiceMonitors
Tells Prometheus Operator which endpoints to scrape:
- Standard Prometheus ServiceMonitors look for pods labeled `app=<service>` and fetch Prometheus `/metrics` on port `8080` (or target metrics port) every 15 seconds.

### 4.2 PrometheusRules
Contains the alerting configurations used by Prometheus Operator:
- Evaluates SLO metrics (availability, error budget burn rates, latency) over moving averages.
- Example: If a service's 2-minute error rate exceeds 5% or latency exceeds 500ms, an alert fires.

### 4.3 AlertmanagerConfig
Defines the routing rules:
- Routes incoming alerts labeled with `severity: critical` to the Rollback Operator webhook endpoint at `http://rollback-operator.default.svc.cluster.local:8090/webhook`.
