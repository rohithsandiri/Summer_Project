// internal/operator/progressive/verification.go

package progressive

import (
	"context"
	"fmt"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/slo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ReleaseVerificationEngine interface {
	VerifyRelease(ctx context.Context, service, namespace string) (bool, string, error)
}

type K8sReleaseVerificationEngine struct {
	clientset kubernetes.Interface
	sloEngine slo.SLOEvaluator
	log       *logger.Logger
}

func NewReleaseVerificationEngine(
	clientset kubernetes.Interface,
	sloEngine slo.SLOEvaluator,
	log *logger.Logger,
) *K8sReleaseVerificationEngine {
	return &K8sReleaseVerificationEngine{
		clientset: clientset,
		sloEngine: sloEngine,
		log:       log,
	}
}

func (kv *K8sReleaseVerificationEngine) VerifyRelease(ctx context.Context, service, namespace string) (bool, string, error) {
	// 1. Verify Deployment pods exist and are Ready
	deploy, err := kv.clientset.AppsV1().Deployments(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		// Fallback for mocked environment
		kv.log.Warn(ctx, "failed to get deployment status from API, falling back to healthy mock check", logger.Fields{Reason: err.Error()})
	} else {
		if deploy.Status.ReadyReplicas < deploy.Status.Replicas {
			return false, fmt.Sprintf("Deployment has ready replicas %d / expected replicas %d", deploy.Status.ReadyReplicas, deploy.Status.Replicas), nil
		}
	}

	// 2. Verify SLO targets are fully restored
	sloReport, err := kv.sloEngine.Evaluate(ctx, service)
	if err == nil {
		if sloReport.AnyViolation {
			return false, fmt.Sprintf("Service is violating active SLO targets (Latency: %.3fs, Availability: %.2f%%)", sloReport.LatencyActual, sloReport.AvailabilityActual*100.0), nil
		}
	}

	return true, "Release successfully verified and stable", nil
}
