// internal/operator/config/config.go
//
// Operator configuration model and validation logic. Loaded from environmental configs.

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type Config struct {
	Port                    string
	OperatorVersion         string
	Policies                []models.Policy
	SLOs                    []models.SLO
	PrometheusAddr          string
	LeaderElectionEnabled   bool
	LeaderElectionID        string
	LeaderElectionNamespace string

	// Dependency-aware batch rollback: when a service is rolled back, roll
	// back tightly-coupled services co-released within BatchWindow too.
	DependencyRollbackEnabled bool
	DependencyBatchWindow     time.Duration
}

func Load() (*Config, error) {
	port := os.Getenv("OPERATOR_PORT")
	if port == "" {
		port = "8090"
	}

	version := os.Getenv("OPERATOR_VERSION")
	if version == "" {
		version = "1.0.0"
	}

	promAddr := os.Getenv("PROMETHEUS_ADDR")
	if promAddr == "" {
		// kube-prometheus-stack installed as release "monitoring" (see
		// scripts/01-setup-cluster.sh) names its Prometheus service this way;
		// prometheus-k8s is the plain kube-prometheus manifests' name and
		// does not exist in this cluster.
		promAddr = "http://monitoring-kube-prometheus-prometheus.monitoring.svc:9090"
	}

	// Initialize default policies.
	// NOTE: AlertName here MUST match the `alert:` name emitted by the
	// PrometheusRule in k8s/prometheus-rules/slo-alerting-rules.yaml — the
	// policy engine matches on (Service, AlertName) exact string equality.
	policies := []models.Policy{
		// ── order-service ──────────────────────────────────────────────
		{
			ID:                "policy-order-error-rate-rollback",
			Service:           "order-service",
			AlertName:         "OrderServiceHighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-order-latency-rollback",
			Service:           "order-service",
			AlertName:         "OrderServiceHighLatency",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-order-no-healthy-pods-restart",
			Service:           "order-service",
			AlertName:         "OrderServiceNoHealthyPods",
			RecommendedAction: models.ActionPrepareRestart,
			CooldownDuration:  3 * time.Minute,
			Timeout:           5 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-order-stuck-rollout-rollback",
			Service:           "order-service",
			AlertName:         "OrderServiceStuckRollout",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-order-traffic-drop-wait",
			Service:           "order-service",
			AlertName:         "OrderServiceTrafficDrop",
			RecommendedAction: models.ActionWait,
			CooldownDuration:  2 * time.Minute,
			Timeout:           5 * time.Minute,
			MaxRetries:        1,
		},
		// ── inventory-service ──────────────────────────────────────────
		{
			ID:                "policy-inventory-error-rate-rollback",
			Service:           "inventory-service",
			AlertName:         "InventoryServiceHighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  10 * time.Minute,
			Timeout:           15 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-inventory-latency-rollback",
			Service:           "inventory-service",
			AlertName:         "InventoryServiceHighLatency",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  10 * time.Minute,
			Timeout:           15 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-inventory-stuck-rollout-rollback",
			Service:           "inventory-service",
			AlertName:         "InventoryServiceStuckRollout",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-inventory-no-healthy-pods-restart",
			Service:           "inventory-service",
			AlertName:         "InventoryServiceNoHealthyPods",
			RecommendedAction: models.ActionPrepareRestart,
			CooldownDuration:  10 * time.Minute,
			Timeout:           15 * time.Minute,
			MaxRetries:        2,
		},
		// ── payment-service ─────────────────────────────────────────────
		{
			ID:                "policy-payment-rollback",
			Service:           "payment-service",
			AlertName:         "PaymentServiceHighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-payment-latency-rollback",
			Service:           "payment-service",
			AlertName:         "PaymentServiceHighLatency",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-payment-stuck-rollout-rollback",
			Service:           "payment-service",
			AlertName:         "PaymentServiceStuckRollout",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-payment-no-healthy-pods-restart",
			Service:           "payment-service",
			AlertName:         "PaymentServiceNoHealthyPods",
			RecommendedAction: models.ActionPrepareRestart,
			CooldownDuration:  3 * time.Minute,
			Timeout:           5 * time.Minute,
			MaxRetries:        3,
		},
	}

	// Initialize SRE Service SLOs as specified in objective specification
	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.999, // 99.9%
			LatencyP95Max:      0.300, // 300ms
			ErrorRateMax:       0.01,  // 1%
			ThroughputMin:      10.0,
		},
		{
			ServiceID:          "inventory-service",
			AvailabilityTarget: 0.9995, // 99.95%
			LatencyP95Max:      0.200,  // 200ms
			ErrorRateMax:       0.005,  // 0.5%
			ThroughputMin:      5.0,
		},
		{
			ServiceID:          "api-gateway",
			AvailabilityTarget: 0.999, // 99.9%
			LatencyP95Max:      0.100, // 100ms
			ErrorRateMax:       0.01,  // 1%
			ThroughputMin:      20.0,
		},
		{
			ServiceID:          "order-service",
			AvailabilityTarget: 0.999, // 99.9%
			LatencyP95Max:      0.250, // 250ms
			ErrorRateMax:       0.01,  // 1%
			ThroughputMin:      10.0,
		},
	}

	// Enabled unless explicitly disabled — co-released contract partners
	// being stranded on a lone rollback is the default failure mode.
	depEnabled := os.Getenv("DEPENDENCY_ROLLBACK_ENABLED") != "false"
	depWindow := 10 * time.Minute
	if v := os.Getenv("DEPENDENCY_BATCH_WINDOW_MINUTES"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins > 0 {
			depWindow = time.Duration(mins) * time.Minute
		}
	}

	leEnabled := os.Getenv("LEADER_ELECTION_ENABLED") == "true"
	leID := os.Getenv("POD_NAME")
	if leID == "" {
		leID = "rollback-operator-local"
	}
	leNamespace := os.Getenv("POD_NAMESPACE")
	if leNamespace == "" {
		leNamespace = "default"
	}

	cfg := &Config{
		Port:                    port,
		OperatorVersion:         version,
		Policies:                policies,
		SLOs:                    slos,
		PrometheusAddr:          promAddr,
		LeaderElectionEnabled:   leEnabled,
		LeaderElectionID:        leID,
		LeaderElectionNamespace: leNamespace,

		DependencyRollbackEnabled: depEnabled,
		DependencyBatchWindow:     depWindow,
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if _, err := strconv.Atoi(c.Port); err != nil {
		return fmt.Errorf("invalid port %q: must be integer", c.Port)
	}
	if len(c.Policies) == 0 {
		return errors.New("no policies configured")
	}
	for _, p := range c.Policies {
		if p.ID == "" {
			return errors.New("policy ID is required")
		}
		if p.Service == "" {
			return fmt.Errorf("service name is required in policy %s", p.ID)
		}
		if p.AlertName == "" {
			return fmt.Errorf("alert name is required in policy %s", p.ID)
		}
		if p.RecommendedAction == "" {
			return fmt.Errorf("recommended action is required in policy %s", p.ID)
		}
		if p.CooldownDuration <= 0 {
			return fmt.Errorf("cooldown duration must be positive in policy %s", p.ID)
		}
	}
	return nil
}
