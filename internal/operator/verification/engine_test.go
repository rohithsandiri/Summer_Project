// internal/operator/verification/engine_test.go

package verification

import (
	"context"
	"testing"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/metrics"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }

func TestVerificationEngineHealthy(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	// 1. Build fake deployment and healthy pods
	serviceName := "payment-service"
	namespace := "cloud-native-platform"

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": serviceName},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:   2,
			UpdatedReplicas: 2,
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-pod-1",
			Namespace: namespace,
			Labels:    map[string]string{"app": serviceName},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: true,
				},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-pod-2",
			Namespace: namespace,
			Labels:    map[string]string{"app": serviceName},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: true,
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(deploy, pod1, pod2)
	verifier := NewK8sVerificationEngine(clientset, m, log)

	ok, err := verifier.Verify(ctx, serviceName, namespace, "trace-123")
	if err != nil {
		t.Fatalf("unexpected verification error: %v", err)
	}

	if !ok {
		t.Errorf("expected verification to pass")
	}
}

func TestVerificationEngineUnhealthyCrashLoop(t *testing.T) {
	log := logger.New("1.0.0")
	m := metrics.New()
	ctx := context.Background()

	serviceName := "payment-service"
	namespace := "cloud-native-platform"

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": serviceName},
			},
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:   2,
			UpdatedReplicas: 2,
		},
	}

	// Pod 2 is in CrashLoopBackOff waiting status
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-pod-1",
			Namespace: namespace,
			Labels:    map[string]string{"app": serviceName},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: true,
				},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-pod-2",
			Namespace: namespace,
			Labels:    map[string]string{"app": serviceName},
		},
		Status: corev1.PodStatus{
			Phase: "Running",
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "app",
					Ready: false,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(deploy, pod1, pod2)
	verifier := NewK8sVerificationEngine(clientset, m, log)

	ok, err := verifier.Verify(ctx, serviceName, namespace, "trace-123")
	if err != nil {
		t.Fatalf("unexpected error during verification check: %v", err)
	}

	if ok {
		t.Errorf("expected verification to fail due to CrashLoopBackOff")
	}
}
