// internal/operator/progressive/verification_test.go

package progressive

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReleaseVerification(t *testing.T) {
	ctx := context.Background()
	log := logger.New("test")
	m := metrics.New()
	promMock := prometheus.NewMockClient()

	slos := []models.SLO{
		{
			ServiceID:          "payment-service",
			AvailabilityTarget: 0.99,
			LatencyP95Max:      0.5,
			ErrorRateMax:       0.05,
		},
	}
	sloEngine := slo.NewEngine(slos, promMock, m)

	// Prepare fake k8s client
	clientset := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-service",
			Namespace: "default",
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      3,
			ReadyReplicas: 3,
		},
	})

	verifier := NewReleaseVerificationEngine(clientset, sloEngine, log)

	// Case 1: Healthy status & healthy SLOs -> Verified
	promMock.AvailabilityValues["payment-service"] = 0.995
	promMock.LatencyValues["payment-service"] = 0.100

	verified, reason, err := verifier.VerifyRelease(ctx, "payment-service", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !verified {
		t.Errorf("expected verified deployment, got unverified: %s", reason)
	}
}
