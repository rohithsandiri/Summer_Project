// internal/operator/prometheus/client.go
//
// Prometheus Query Engine using the official Prometheus HTTP client API.
// Allows query evaluation for Golden Signals: Latency, Error Rate, Traffic, CPU, Memory.

package prometheus

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
)

// PrometheusClient defines interfaces to query PromQL golden signals.
type PrometheusClient interface {
	Query(ctx context.Context, query string, ts time.Time) (float64, error)
	GetLatency(ctx context.Context, service string) (float64, error)
	GetAvailability(ctx context.Context, service string) (float64, error)
	GetErrorRate(ctx context.Context, service string) (float64, error)
	GetTraffic(ctx context.Context, service string) (float64, error)
	GetCPU(ctx context.Context, service string) (float64, error)
	GetMemory(ctx context.Context, service string) (float64, error)
}

type Client struct {
	v1API v1.API
	m     *metrics.OperatorMetrics
}

func NewClient(address string, m *metrics.OperatorMetrics) (*Client, error) {
	cfg := api.Config{
		Address: address,
	}
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &Client{
		v1API: v1.NewAPI(client),
		m:     m,
	}, nil
}

func (c *Client) Query(ctx context.Context, query string, ts time.Time) (float64, error) {
	c.m.PrometheusQueriesTotal.Inc()

	val, _, err := c.v1API.Query(ctx, query, ts)
	if err != nil {
		return 0, err
	}

	switch v := val.(type) {
	case model.Vector:
		if len(v) == 0 {
			return 0, nil
		}
		return float64(v[0].Value), nil
	case *model.Scalar:
		return float64(v.Value), nil
	default:
		return 0, fmt.Errorf("unexpected model value type: %T", val)
	}
}

func (c *Client) GetLatency(ctx context.Context, service string) (float64, error) {
	// Calculate p95 latency in seconds over the last 5m
	query := fmt.Sprintf(`histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{service="%s"}[5m])) by (le))`, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 0, nil
	}
	return val, nil
}

func (c *Client) GetAvailability(ctx context.Context, service string) (float64, error) {
	// Availability defined as: non-5xx requests / total requests over 5m
	query := fmt.Sprintf(`sum(rate(http_requests_total{service="%s", status!~"5.."}[5m])) / (sum(rate(http_requests_total{service="%s"}[5m])) or vector(1))`, service, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 1.0, nil // Default to 100% available if no traffic
	}
	return val, nil
}

func (c *Client) GetErrorRate(ctx context.Context, service string) (float64, error) {
	// Error rate defined as: 5xx requests / total requests over 5m
	query := fmt.Sprintf(`sum(rate(http_requests_total{service="%s", status=~"5.."}[5m])) / (sum(rate(http_requests_total{service="%s"}[5m])) or vector(1))`, service, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 0, nil
	}
	return val, nil
}

func (c *Client) GetTraffic(ctx context.Context, service string) (float64, error) {
	// Traffic in requests per second over 5m
	query := fmt.Sprintf(`sum(rate(http_requests_total{service="%s"}[5m]))`, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 0, nil
	}
	return val, nil
}

func (c *Client) GetCPU(ctx context.Context, service string) (float64, error) {
	// CPU usage core rate
	query := fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{container="%s"}[5m]))`, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 0, nil
	}
	return val, nil
}

func (c *Client) GetMemory(ctx context.Context, service string) (float64, error) {
	// Memory usage bytes
	query := fmt.Sprintf(`sum(container_memory_working_set_bytes{container="%s"})`, service)
	val, err := c.Query(ctx, query, time.Now())
	if err != nil {
		return 0, err
	}
	if math.IsNaN(val) {
		return 0, nil
	}
	return val, nil
}
