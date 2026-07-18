// internal/operator/verification/engine.go
//
// Verification Engine validating Kubernetes pod and deployment readiness.
// Confirms that all replicas are running, ready, and not in CrashLoopBackOff.

package verification

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/operator/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerificationEngine defines the check interface to confirm service recovery.
type VerificationEngine interface {
	Verify(ctx context.Context, service string, namespace string, traceID string) (bool, error)
}

// SLOVerifier confirms the violated SLO itself has recovered, not just that
// pods are healthy. Pod health alone is a false signal after a rollback that
// lands on a revision with a latent fault: pods answer their probes while the
// service still breaches its SLO. Implemented by engines that hold a
// Prometheus client; the incident manager type-asserts for it so existing
// VerificationEngine implementations (and test mocks) keep working unchanged.
type SLOVerifier interface {
	VerifySLO(ctx context.Context, service string, alertName string, traceID string) (bool, error)
}

// K8sVerificationEngine implements VerificationEngine using Kubernetes client-go APIs.
type K8sVerificationEngine struct {
	clientset  kubernetes.Interface
	promClient prometheus.PrometheusClient
	m          *metrics.OperatorMetrics
	log        *logger.Logger
}

func NewK8sVerificationEngine(
	clientset kubernetes.Interface,
	m *metrics.OperatorMetrics,
	log *logger.Logger,
) *K8sVerificationEngine {
	return &K8sVerificationEngine{
		clientset: clientset,
		m:         m,
		log:       log,
	}
}

// WithPrometheus enables SLO-aware verification by attaching a Prometheus
// client. Without it, VerifySLO trusts pod health alone (returns true).
func (ve *K8sVerificationEngine) WithPrometheus(pc prometheus.PrometheusClient) *K8sVerificationEngine {
	ve.promClient = pc
	return ve
}

// VerifySLO reports whether the alert that triggered the incident has stopped
// firing in Prometheus. The ALERTS series covers every SLO type uniformly
// (error rate, latency, availability, stuck rollout) without re-encoding each
// rule's expression here, and `or vector(0)` keeps the query non-empty so a
// fully-resolved alert reads as 0.
func (ve *K8sVerificationEngine) VerifySLO(ctx context.Context, service string, alertName string, traceID string) (bool, error) {
	if ve.promClient == nil || alertName == "" {
		return true, nil
	}

	f := logger.Fields{
		TraceID:   traceID,
		Service:   service,
		AlertName: alertName,
	}

	query := fmt.Sprintf(`count(ALERTS{alertname=%q, alertstate="firing"}) or vector(0)`, alertName)
	firing, err := ve.promClient.Query(ctx, query, time.Now())
	if err != nil {
		// Prometheus being unreachable must not wedge recovery: report the
		// error but do not fail verification on infrastructure flakiness.
		ve.log.Warn(ctx, "slo verification query failed, falling back to pod-health result", f, "error", err.Error())
		return true, nil
	}

	if firing > 0 {
		ve.log.Warn(ctx, "slo alert still firing after rollback, verification not yet passed", f, "firing_instances", firing)
		return false, nil
	}

	ve.log.Info(ctx, "slo verification passed: triggering alert no longer firing", f)
	return true, nil
}

func (ve *K8sVerificationEngine) Verify(ctx context.Context, service string, namespace string, traceID string) (bool, error) {
	ve.m.VerificationsTotal.WithLabelValues(service, "started").Inc()

	f := logger.Fields{
		TraceID: traceID,
		Service: service,
	}

	ve.log.Info(ctx, "starting kubernetes health verification checks", f)

	// 1. Get deployment and check replica availability
	deploy, err := ve.clientset.AppsV1().Deployments(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, fmt.Errorf("failed to get deployment: %w", err)
	}

	if deploy.Spec.Replicas == nil {
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, fmt.Errorf("deployment Spec.Replicas is nil")
	}

	desiredReplicas := *deploy.Spec.Replicas
	readyReplicas := deploy.Status.ReadyReplicas
	updatedReplicas := deploy.Status.UpdatedReplicas

	if readyReplicas < desiredReplicas || updatedReplicas < desiredReplicas {
		ve.log.Warn(ctx, "replica count mismatch during verification", f,
			"desired", desiredReplicas, "ready", readyReplicas, "updated", updatedReplicas)
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, nil
	}

	// 2. Fetch pods using selector
	selector, err := metav1.LabelSelectorAsSelector(deploy.Spec.Selector)
	if err != nil {
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, fmt.Errorf("failed to compile selector: %w", err)
	}

	pods, err := ve.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		ve.log.Warn(ctx, "no pods found matching deployment selector", f)
		ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
		ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
		return false, nil
	}

	// 3. Inspect status details of each pod (detect CrashLoopBackOff, ImagePullBackOff, etc.)
	for _, pod := range pods.Items {
		// Pod Phase check
		if pod.Status.Phase != "Running" {
			ve.log.Warn(ctx, "pod is not in running state", f, "pod", pod.Name, "phase", pod.Status.Phase)
			ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
			ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
			return false, nil
		}

		// Container status check
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				ve.log.Warn(ctx, "pod container is not ready", f, "pod", pod.Name, "container", status.Name)
				ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
				ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
				return false, nil
			}

			// CrashLoop / Wait reasons
			if status.State.Waiting != nil {
				reason := status.State.Waiting.Reason
				if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
					ve.log.Warn(ctx, "pod container waiting with critical state", f, "pod", pod.Name, "reason", reason)
					ve.m.VerificationFailuresTotal.WithLabelValues(service).Inc()
					ve.m.VerificationsTotal.WithLabelValues(service, "failed").Inc()
					return false, nil
				}
			}
		}
	}

	// All checks passed
	ve.m.VerificationsTotal.WithLabelValues(service, "success").Inc()
	ve.log.Info(ctx, "kubernetes health verification checks passed successfully", f)
	return true, nil
}
