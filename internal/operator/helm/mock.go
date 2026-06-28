// internal/operator/helm/mock.go
//
// Mock implementation of the HelmManager for testing recovery flows.

package helm

import (
	"fmt"

	"helm.sh/helm/v3/pkg/release"
)

type MockHelmClient struct {
	Releases          map[string][]*release.Release // key: namespace/releaseName
	CurrentVersion    map[string]int                // key: namespace/releaseName
	FailOnRollback    bool
	FailOnHistory     bool
	FailWithTransient bool
}

func NewMockHelmClient() *MockHelmClient {
	return &MockHelmClient{
		Releases:       make(map[string][]*release.Release),
		CurrentVersion: make(map[string]int),
	}
}

func (m *MockHelmClient) key(ns, name string) string {
	return fmt.Sprintf("%s/%s", ns, name)
}

func (m *MockHelmClient) ListReleases(namespace string) ([]*release.Release, error) {
	var list []*release.Release
	for k, rels := range m.Releases {
		if len(rels) > 0 {
			list = append(list, rels[len(rels)-1])
		}
		_ = k
	}
	return list, nil
}

func (m *MockHelmClient) GetRelease(namespace, name string) (*release.Release, error) {
	k := m.key(namespace, name)
	history, ok := m.Releases[k]
	if !ok || len(history) == 0 {
		return nil, fmt.Errorf("release: not found %s", name)
	}
	return history[len(history)-1], nil
}

func (m *MockHelmClient) GetStatus(namespace, name string) (*release.Release, error) {
	return m.GetRelease(namespace, name)
}

func (m *MockHelmClient) GetHistory(namespace, name string) ([]*release.Release, error) {
	if m.FailOnHistory {
		return nil, fmt.Errorf("failed to fetch release history")
	}

	k := m.key(namespace, name)
	history, ok := m.Releases[k]
	if !ok {
		return nil, fmt.Errorf("release: not found %s", name)
	}
	return history, nil
}

func (m *MockHelmClient) GetCurrentRevision(namespace, name string) (int, error) {
	k := m.key(namespace, name)
	rev, ok := m.CurrentVersion[k]
	if !ok {
		return 0, fmt.Errorf("release: not found %s", name)
	}
	return rev, nil
}

func (m *MockHelmClient) GetLastHealthyRevision(namespace, name string) (int, error) {
	k := m.key(namespace, name)
	history, ok := m.Releases[k]
	if !ok {
		return 0, fmt.Errorf("release: not found %s", name)
	}

	curr := m.CurrentVersion[k]
	for i := len(history) - 1; i >= 0; i-- {
		rel := history[i]
		if rel.Version == curr {
			continue
		}
		if rel.Info != nil && rel.Info.Status == release.StatusDeployed {
			return rel.Version, nil
		}
	}
	return 0, fmt.Errorf("no healthy historical revision found")
}

func (m *MockHelmClient) Rollback(namespace, name string, targetRevision int) error {
	if m.FailOnRollback {
		if m.FailWithTransient {
			return fmt.Errorf("transient connection timeout error")
		}
		return fmt.Errorf("invalid namespace: %s does not exist", namespace)
	}

	k := m.key(namespace, name)
	history, ok := m.Releases[k]
	if !ok {
		return fmt.Errorf("release not found: %s", name)
	}

	// Find the targeted release revision
	var targetRel *release.Release
	for _, rel := range history {
		if rel.Version == targetRevision {
			targetRel = rel
			break
		}
	}

	if targetRel == nil {
		return fmt.Errorf("revision not found: %d", targetRevision)
	}

	// Create rolled back revision
	newRevNum := m.CurrentVersion[k] + 1
	m.CurrentVersion[k] = newRevNum

	newRel := &release.Release{
		Name:      targetRel.Name,
		Namespace: targetRel.Namespace,
		Version:   newRevNum,
		Info: &release.Info{
			Status:      release.StatusDeployed,
			Description: fmt.Sprintf("Rolled back to v%d", targetRevision),
		},
		Chart: targetRel.Chart,
	}

	m.Releases[k] = append(m.Releases[k], newRel)
	return nil
}
