// internal/operator/argo/mock.go

package argo

import (
	"context"
	"fmt"
)

type MockArgoClient struct {
	Rollouts map[string]*RolloutDetails
}

func NewMockArgoClient() *MockArgoClient {
	return &MockArgoClient{
		Rollouts: make(map[string]*RolloutDetails),
	}
}

func (m *MockArgoClient) GetRollout(ctx context.Context, name, namespace string) (*RolloutDetails, error) {
	key := fmt.Sprintf("%s/%s", namespace, name)
	ro, ok := m.Rollouts[key]
	if !ok {
		// Populate dynamic mock rollout
		ro = &RolloutDetails{
			Name:            name,
			Namespace:       namespace,
			Strategy:        "Canary",
			CurrentWeight:   0,
			DesiredWeight:   100,
			Paused:          false,
			Aborted:         false,
			CurrentRevision: 1,
		}
		m.Rollouts[key] = ro
	}
	return ro, nil
}

func (m *MockArgoClient) PauseRollout(ctx context.Context, name, namespace string) error {
	ro, err := m.GetRollout(ctx, name, namespace)
	if err != nil {
		return err
	}
	ro.Paused = true
	return nil
}

func (m *MockArgoClient) ResumeRollout(ctx context.Context, name, namespace string) error {
	ro, err := m.GetRollout(ctx, name, namespace)
	if err != nil {
		return err
	}
	ro.Paused = false
	return nil
}

func (m *MockArgoClient) AbortRollout(ctx context.Context, name, namespace string) error {
	ro, err := m.GetRollout(ctx, name, namespace)
	if err != nil {
		return err
	}
	ro.Aborted = true
	ro.Paused = true
	return nil
}

func (m *MockArgoClient) PromoteRollout(ctx context.Context, name, namespace string) error {
	ro, err := m.GetRollout(ctx, name, namespace)
	if err != nil {
		return err
	}
	ro.CurrentWeight = 100
	ro.Paused = false
	return nil
}
