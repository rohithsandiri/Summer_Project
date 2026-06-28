// internal/shared/config/config.go
//
// WHY THIS EXISTS:
//   12-factor apps load all config from environment variables. Centralising
//   config loading means every service follows the same pattern, and future
//   phases (Helm, Kubernetes ConfigMaps, Vault) can inject config uniformly.
//
// VALIDATION — FAIL FAST DESIGN:
//   A misconfigured service that starts silently is dangerous: it serves
//   traffic while silently calling the wrong downstream URL, or it listens on
//   the wrong port and fails health checks only after Kubernetes routes traffic.
//
//   Load() validates all critical fields and returns a descriptive error
//   listing every problem found (not just the first). The caller (main.go)
//   calls log.Fatal on error — the service crashes at startup instead of
//   silently misbehaving. This is the "fail fast" principle.

package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for a microservice.
type Config struct {
	// ServiceName identifies this service in logs and metrics labels.
	ServiceName string

	// Version is the service binary version, injected at build time via ldflags.
	Version string

	// Port is the TCP port this service listens on (e.g. "8080").
	Port string

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration

	// LogLevel controls log verbosity: "debug", "info", "warn", "error".
	LogLevel string

	// InventoryServiceURL is the base URL of the Inventory Service.
	InventoryServiceURL string

	// PaymentServiceURL is the base URL of the Payment Service.
	PaymentServiceURL string

	// GatewayPort is the port the API gateway listens on.
	GatewayPort string

	// OrderServiceURL is the base URL of the Order Service.
	OrderServiceURL string

	// ErrorRate is a chaos-engineering knob (0.0–1.0).
	ErrorRate float64

	// LatencyMS adds artificial latency in milliseconds per request.
	LatencyMS float64

	// RedisURL is the host:port address of the Redis instance.
	RedisURL string

	// RedisPassword is the optional authentication password for Redis.
	RedisPassword string

	// RateLimitRPS is the rate-limiter requests per second limit.
	RateLimitRPS float64

	// RateLimitBurst is the rate-limiter burst size.
	RateLimitBurst int

	// OTelExporterEndpoint is the OTLP/HTTP endpoint (host:port).
	OTelExporterEndpoint string
}

// LoadOptions controls which validations are applied.
type LoadOptions struct {
	RequireInventoryURL bool
	RequirePaymentURL   bool
	RequireOrderURL     bool
	RequireRedis        bool
}

// Load reads configuration from environment variables and validates it.
func Load(serviceName string, opts LoadOptions) (Config, error) {
	cfg := Config{
		ServiceName:          serviceName,
		Version:              getEnv("SERVICE_VERSION", "dev"),
		Port:                 getEnv("PORT", "8080"),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
		InventoryServiceURL:  getEnv("INVENTORY_SERVICE_URL", "http://inventory-service:8081"),
		PaymentServiceURL:    getEnv("PAYMENT_SERVICE_URL", "http://payment-service:8082"),
		OrderServiceURL:      getEnv("ORDER_SERVICE_URL", "http://order-service:8080"),
		GatewayPort:          getEnv("GATEWAY_PORT", "8000"),
		ErrorRate:            getEnvFloat("ERROR_RATE", 0.0),
		LatencyMS:            getEnvFloat("LATENCY_MS", 0.0),
		RedisURL:             getEnv("REDIS_URL", "redis:6379"),
		RedisPassword:        getEnv("REDIS_PASSWORD", ""),
		RateLimitRPS:         getEnvFloat("RATE_LIMIT_RPS", 10.0),
		RateLimitBurst:       getEnvInt("RATE_LIMIT_BURST", 20),
		OTelExporterEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "tempo:4318"),
	}

	cfg.ReadTimeout = getEnvDuration("READ_TIMEOUT", 15*time.Second)
	cfg.WriteTimeout = getEnvDuration("WRITE_TIMEOUT", 30*time.Second)

	var errs []string

	// Validate port
	if err := validatePort(cfg.Port); err != nil {
		errs = append(errs, fmt.Sprintf("PORT=%q: %v", cfg.Port, err))
	}

	// Validate timeout ranges
	if cfg.ReadTimeout < time.Second || cfg.ReadTimeout > 5*time.Minute {
		errs = append(errs, "READ_TIMEOUT must be between 1s and 5m")
	}
	if cfg.WriteTimeout < time.Second || cfg.WriteTimeout > 5*time.Minute {
		errs = append(errs, "WRITE_TIMEOUT must be between 1s and 5m")
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.LogLevel)] {
		errs = append(errs, fmt.Sprintf("LOG_LEVEL=%q is invalid; must be one of: debug, info, warn, error", cfg.LogLevel))
	}

	// Validate error rate range
	if cfg.ErrorRate < 0.0 || cfg.ErrorRate > 1.0 {
		errs = append(errs, fmt.Sprintf("ERROR_RATE=%.2f is out of range; must be 0.0–1.0", cfg.ErrorRate))
	}

	// Validate rate limiter bounds
	if cfg.RateLimitRPS <= 0.0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_RPS=%.2f must be positive", cfg.RateLimitRPS))
	}
	if cfg.RateLimitBurst <= 0 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_BURST=%d must be positive", cfg.RateLimitBurst))
	}

	// Validate downstream URLs when required
	if opts.RequireInventoryURL {
		if err := validateURL(cfg.InventoryServiceURL); err != nil {
			errs = append(errs, fmt.Sprintf("INVENTORY_SERVICE_URL=%q: %v", cfg.InventoryServiceURL, err))
		}
	}
	if opts.RequirePaymentURL {
		if err := validateURL(cfg.PaymentServiceURL); err != nil {
			errs = append(errs, fmt.Sprintf("PAYMENT_SERVICE_URL=%q: %v", cfg.PaymentServiceURL, err))
		}
	}
	if opts.RequireOrderURL {
		if err := validateURL(cfg.OrderServiceURL); err != nil {
			errs = append(errs, fmt.Sprintf("ORDER_SERVICE_URL=%q: %v", cfg.OrderServiceURL, err))
		}
	}

	// Validate Redis configuration if required
	if opts.RequireRedis {
		if cfg.RedisURL == "" {
			errs = append(errs, "REDIS_URL must not be empty when Redis is required")
		} else {
			parts := strings.Split(cfg.RedisURL, ":")
			if len(parts) != 2 {
				errs = append(errs, fmt.Sprintf("REDIS_URL=%q is invalid; must be host:port", cfg.RedisURL))
			} else {
				if err := validatePort(parts[1]); err != nil {
					errs = append(errs, fmt.Sprintf("REDIS_URL port: %v", err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.New("configuration errors:\n  - " + strings.Join(errs, "\n  - "))
	}

	return cfg, nil
}

// validatePort checks that a port string is a valid TCP port number.
func validatePort(port string) error {
	p, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("must be a number, got %q", port)
	}
	if p < 1 || p > 65535 {
		return fmt.Errorf("must be in range 1–65535, got %d", p)
	}
	return nil
}

// validateURL checks that a URL string is parseable and has http/https scheme.
func validateURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("must not be empty")
	}
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("not a valid URL: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("host must not be empty")
	}
	return nil
}

// getEnv returns the env var value or a default.
func getEnv(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

// getEnvFloat parses a float64 env var or returns the default.
func getEnvFloat(key string, defaultVal float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

// getEnvInt parses an int env var or returns the default.
func getEnvInt(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}

// getEnvDuration parses a time.Duration env var or returns default.
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultVal
	}
	return d
}
