// internal/operator/prometheus/client_test.go

package prometheus

import (
	"context"
	"testing"
)

func TestMockPrometheusClient(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()

	m.LatencyValues["payment-service"] = 0.450
	m.AvailabilityValues["payment-service"] = 0.985
	m.ErrorRateValues["payment-service"] = 0.02
	m.TrafficValues["payment-service"] = 15.0
	m.CPUValues["payment-service"] = 0.75
	m.MemoryValues["payment-service"] = 128 * 1024 * 1024

	lat, err := m.GetLatency(ctx, "payment-service")
	if err != nil || lat != 0.450 {
		t.Errorf("expected p95 latency 0.450, got %v (err: %v)", lat, err)
	}

	avail, err := m.GetAvailability(ctx, "payment-service")
	if err != nil || avail != 0.985 {
		t.Errorf("expected availability 0.985, got %v (err: %v)", avail, err)
	}

	errRate, err := m.GetErrorRate(ctx, "payment-service")
	if err != nil || errRate != 0.02 {
		t.Errorf("expected error rate 0.02, got %v (err: %v)", errRate, err)
	}

	traffic, err := m.GetTraffic(ctx, "payment-service")
	if err != nil || traffic != 15.0 {
		t.Errorf("expected traffic 15.0, got %v (err: %v)", traffic, err)
	}

	cpu, err := m.GetCPU(ctx, "payment-service")
	if err != nil || cpu != 0.75 {
		t.Errorf("expected cpu 0.75, got %v (err: %v)", cpu, err)
	}

	mem, err := m.GetMemory(ctx, "payment-service")
	if err != nil || mem != 128*1024*1024 {
		t.Errorf("expected memory 134217728, got %v (err: %v)", mem, err)
	}
}
