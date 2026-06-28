// internal/operator/argo/client.go

package argo

import (
	"context"
	"fmt"

	argoclientset "github.com/argoproj/argo-rollouts/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type RolloutDetails struct {
	Name            string
	Namespace       string
	Strategy        string // "Canary" | "BlueGreen"
	CurrentWeight   int32
	DesiredWeight   int32
	Paused          bool
	Aborted         bool
	CurrentRevision int
	ActiveService   string
	PreviewService  string
}

type RolloutManager interface {
	GetRollout(ctx context.Context, name, namespace string) (*RolloutDetails, error)
	PauseRollout(ctx context.Context, name, namespace string) error
	ResumeRollout(ctx context.Context, name, namespace string) error
	AbortRollout(ctx context.Context, name, namespace string) error
	PromoteRollout(ctx context.Context, name, namespace string) error
}

type ArgoClient struct {
	clientset argoclientset.Interface
}

func NewArgoClient(config *rest.Config) (*ArgoClient, error) {
	cs, err := argoclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create argo clientset: %w", err)
	}
	return &ArgoClient{clientset: cs}, nil
}

func (a *ArgoClient) GetRollout(ctx context.Context, name, namespace string) (*RolloutDetails, error) {
	ro, err := a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	strategy := "Canary"
	var active, preview string
	if ro.Spec.Strategy.BlueGreen != nil {
		strategy = "Blue-Green"
		active = ro.Spec.Strategy.BlueGreen.ActiveService
		preview = ro.Spec.Strategy.BlueGreen.PreviewService
	}

	var currentWeight, desiredWeight int32
	if ro.Status.CurrentStepIndex != nil {
		currentWeight = *ro.Status.CurrentStepIndex
		desiredWeight = 100
	}

	return &RolloutDetails{
		Name:            ro.Name,
		Namespace:       ro.Namespace,
		Strategy:        strategy,
		CurrentWeight:   currentWeight,
		DesiredWeight:   desiredWeight,
		Paused:          ro.Status.ControllerPause,
		Aborted:         ro.Status.Abort,
		CurrentRevision: int(ro.Generation),
		ActiveService:   active,
		PreviewService:  preview,
	}, nil
}

func (a *ArgoClient) PauseRollout(ctx context.Context, name, namespace string) error {
	ro, err := a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	ro.Spec.Paused = true
	_, err = a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Update(ctx, ro, metav1.UpdateOptions{})
	return err
}

func (a *ArgoClient) ResumeRollout(ctx context.Context, name, namespace string) error {
	ro, err := a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	ro.Spec.Paused = false
	_, err = a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Update(ctx, ro, metav1.UpdateOptions{})
	return err
}

func (a *ArgoClient) AbortRollout(ctx context.Context, name, namespace string) error {
	ro, err := a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// Argo Rollouts supports aborting rollouts via setting Spec status
	ro.Spec.Paused = true
	// Simulating abort indicator
	if ro.Annotations == nil {
		ro.Annotations = make(map[string]string)
	}
	ro.Annotations["argo-rollouts.argoproj.io/abort"] = "true"
	_, err = a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Update(ctx, ro, metav1.UpdateOptions{})
	return err
}

func (a *ArgoClient) PromoteRollout(ctx context.Context, name, namespace string) error {
	ro, err := a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// Argo rollouts supports promotion by clearing pause step requirements
	ro.Spec.Paused = false
	if ro.Annotations == nil {
		ro.Annotations = make(map[string]string)
	}
	ro.Annotations["argo-rollouts.argoproj.io/promote-full"] = "true"
	_, err = a.clientset.ArgoprojV1alpha1().Rollouts(namespace).Update(ctx, ro, metav1.UpdateOptions{})
	return err
}
