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
		promAddr = "http://prometheus-k8s.monitoring.svc:9090"
	}

	// Initialize default policies
	policies := []models.Policy{
		{
			ID:                "policy-payment-rollback",
			Service:           "payment-service",
			AlertName:         "HighErrorRate",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  5 * time.Minute,
			Timeout:           10 * time.Minute,
			MaxRetries:        3,
		},
		{
			ID:                "policy-gateway-wait",
			Service:           "api-gateway",
			AlertName:         "HighLatency",
			RecommendedAction: models.ActionWait,
			CooldownDuration:  2 * time.Minute,
			Timeout:           5 * time.Minute,
			MaxRetries:        1,
		},
		{
			ID:                "policy-inventory-rollback",
			Service:           "inventory-service",
			AlertName:         "PodCrashLoop",
			RecommendedAction: models.ActionPrepareRollback,
			CooldownDuration:  10 * time.Minute,
			Timeout:           15 * time.Minute,
			MaxRetries:        2,
		},
		{
			ID:                "policy-order-restart",
			Service:           "order-service",
			AlertName:         "ServiceDown",
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
