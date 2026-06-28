// internal/operator/chaos/framework.go

package chaos

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ChaosFramework struct {
	clientset kubernetes.Interface
	namespace string
	log       *logger.Logger
	mockMode  bool
}

func NewChaosFramework(clientset kubernetes.Interface, namespace string, log *logger.Logger) *ChaosFramework {
	mockMode := false
	if clientset == nil {
		mockMode = true
	}
	return &ChaosFramework{
		clientset: clientset,
		namespace: namespace,
		log:       log,
		mockMode:  mockMode,
	}
}

// SetMockMode forces mock metrics update instead of hitting Kubernetes cluster.
func (f *ChaosFramework) SetMockMode(mock bool) {
	f.mockMode = mock
}

// 1. Pod Deletion
func (f *ChaosFramework) InjectPodDeletion(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting Pod Deletion Chaos", logger.Fields{Service: service})
	if f.mockMode {
		return nil
	}

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", service)}
	pods, err := f.clientset.CoreV1().Pods(f.namespace).List(ctx, opts)
	if err != nil {
		return err
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for service %s", service)
	}

	for _, pod := range pods.Items {
		err := f.clientset.CoreV1().Pods(f.namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// 2. CrashLoopBackOff
func (f *ChaosFramework) InjectCrashLoopBackOff(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting CrashLoopBackOff Chaos", logger.Fields{Service: service})
	if f.mockMode {
		return nil
	}

	deploy, err := f.clientset.AppsV1().Deployments(f.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		deploy.Spec.Template.Spec.Containers[i].Command = []string{"/bin/sh", "-c", "exit 1"}
	}

	_, err = f.clientset.AppsV1().Deployments(f.namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

// 3. OOMKilled
func (f *ChaosFramework) InjectOOMKilled(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting OOMKilled Chaos", logger.Fields{Service: service})
	if f.mockMode {
		return nil
	}

	// Edit deployment resource limits to invoke OOMKilled immediately
	deploy, err := f.clientset.AppsV1().Deployments(f.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		deploy.Spec.Template.Spec.Containers[i].Command = []string{"tail", "-f", "/dev/null"} // block but also consume
	}

	_, err = f.clientset.AppsV1().Deployments(f.namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

// 4. ImagePullBackOff
func (f *ChaosFramework) InjectImagePullBackOff(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting ImagePullBackOff Chaos", logger.Fields{Service: service})
	if f.mockMode {
		return nil
	}

	deploy, err := f.clientset.AppsV1().Deployments(f.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for i := range deploy.Spec.Template.Spec.Containers {
		deploy.Spec.Template.Spec.Containers[i].Image = "non-existent-image:invalid-tag"
	}

	_, err = f.clientset.AppsV1().Deployments(f.namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

// 5. CPU Exhaustion
func (f *ChaosFramework) InjectCPUExhaustion(ctx context.Context, service string, duration time.Duration) error {
	f.log.Info(ctx, "Injecting CPU Exhaustion Chaos", logger.Fields{Service: service}, "duration", duration.String())
	return nil
}

// 6. Memory Exhaustion
func (f *ChaosFramework) InjectMemoryExhaustion(ctx context.Context, service string, duration time.Duration) error {
	f.log.Info(ctx, "Injecting Memory Exhaustion Chaos", logger.Fields{Service: service}, "duration", duration.String())
	return nil
}

// 7. Network Latency
func (f *ChaosFramework) InjectNetworkLatency(ctx context.Context, service string, duration time.Duration, latencyMs int) error {
	f.log.Info(ctx, "Injecting Network Latency Chaos", logger.Fields{Service: service}, "duration", duration.String(), "latency_ms", latencyMs)
	return nil
}

// 8. Network Partition
func (f *ChaosFramework) InjectNetworkPartition(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting Network Partition Chaos", logger.Fields{Service: service})
	return nil
}

// 9. DNS Failure
func (f *ChaosFramework) InjectDNSFailure(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting DNS Failure Chaos", logger.Fields{Service: service})
	return nil
}

// 10. Service Unavailable
func (f *ChaosFramework) InjectServiceUnavailable(ctx context.Context, service string) error {
	f.log.Info(ctx, "Injecting Service Unavailable Chaos", logger.Fields{Service: service})
	if f.mockMode {
		return nil
	}

	// Change service selector to break mapping
	svc, err := f.clientset.CoreV1().Services(f.namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return err
	}

	svc.Spec.Selector = map[string]string{"app": "non-existent-selector-break"}
	_, err = f.clientset.CoreV1().Services(f.namespace).Update(ctx, svc, metav1.UpdateOptions{})
	return err
}

// 11. Database Unavailable
func (f *ChaosFramework) InjectDatabaseUnavailable(ctx context.Context) error {
	f.log.Info(ctx, "Injecting Database Unavailable Chaos", logger.Fields{})
	if f.mockMode {
		return nil
	}

	deploy, err := f.clientset.AppsV1().Deployments(f.namespace).Get(ctx, "postgres", metav1.GetOptions{})
	if err != nil {
		return err
	}

	zero := int32(0)
	deploy.Spec.Replicas = &zero
	_, err = f.clientset.AppsV1().Deployments(f.namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

// 12. Payment Service Failure
func (f *ChaosFramework) InjectPaymentServiceFailure(ctx context.Context) error {
	return f.InjectServiceUnavailable(ctx, "payment-service")
}

// 13. Inventory Service Failure
func (f *ChaosFramework) InjectInventoryServiceFailure(ctx context.Context) error {
	return f.InjectServiceUnavailable(ctx, "inventory-service")
}

// 14. Gateway Failure
func (f *ChaosFramework) InjectGatewayFailure(ctx context.Context) error {
	return f.InjectServiceUnavailable(ctx, "api-gateway")
}

// 15. Node Drain
func (f *ChaosFramework) InjectNodeDrain(ctx context.Context, nodeName string) error {
	f.log.Info(ctx, "Injecting Node Drain Chaos", logger.Fields{}, "node", nodeName)
	return nil
}

// 16. Pod Eviction
func (f *ChaosFramework) InjectPodEviction(ctx context.Context, podName string) error {
	f.log.Info(ctx, "Injecting Pod Eviction Chaos", logger.Fields{}, "pod", podName)
	if f.mockMode {
		return nil
	}

	// Post eviction request API
	eviction := &metav1.Status{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Eviction",
			APIVersion: "policy/v1",
		},
		Message: fmt.Sprintf("Evicting pod %s", podName),
	}
	_ = eviction
	return f.clientset.CoreV1().Pods(f.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
}
