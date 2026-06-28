// internal/operator/prometheus/mock.go
//
// Mock implementation of PrometheusClient for tests.

package prometheus

import (
	"context"
	"time"
)

type MockPrometheusClient struct {
	LatencyValues      map[string]float64
	AvailabilityValues map[string]float64
	ErrorRateValues    map[string]float64
	TrafficValues      map[string]float64
	CPUValues          map[string]float64
	MemoryValues       map[string]float64
}

func NewMockClient() *MockPrometheusClient {
	return &MockPrometheusClient{
		LatencyValues:      make(map[string]float64),
		AvailabilityValues: make(map[string]float64),
		ErrorRateValues:    make(map[string]float64),
		TrafficValues:      make(map[string]float64),
		CPUValues:          make(map[string]float64),
		MemoryValues:       make(map[string]float64),
	}
}

func (m *MockPrometheusClient) Query(ctx context.Context, query string, ts time.Time) (float64, error) {
	return 0, nil
}

func (m *MockPrometheusClient) GetLatency(ctx context.Context, service string) (float64, error) {
	if val, ok := m.LatencyValues[service]; ok {
		return val, nil
	}
	return 0.05, nil // 50ms default
}

func (m *MockPrometheusClient) GetAvailability(ctx context.Context, service string) (float64, error) {
	if val, ok := m.AvailabilityValues[service]; ok {
		return val, nil
	}
	return 0.9995, nil // 99.95% default
}

func (m *MockPrometheusClient) GetErrorRate(ctx context.Context, service string) (float64, error) {
	if val, ok := m.ErrorRateValues[service]; ok {
		return val, nil
	}
	return 0.0005, nil // 0.05% default
}

func (m *MockPrometheusClient) GetTraffic(ctx context.Context, service string) (float64, error) {
	if val, ok := m.TrafficValues[service]; ok {
		return val, nil
	}
	return 100.0, nil // 100 rps default
}

func (m *MockPrometheusClient) GetCPU(ctx context.Context, service string) (float64, error) {
	if val, ok := m.CPUValues[service]; ok {
		return val, nil
	}
	return 0.1, nil // 0.1 cores default
}

func (m *MockPrometheusClient) GetMemory(ctx context.Context, service string) (float64, error) {
	if val, ok := m.MemoryValues[service]; ok {
		return val, nil
	}
	return 64 * 1024 * 1024, nil // 64MB default
}
