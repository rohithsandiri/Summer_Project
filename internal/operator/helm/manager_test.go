// internal/operator/helm/manager_test.go

package helm

import (
	"testing"

	"helm.sh/helm/v3/pkg/release"
)

func TestMockHelmClientHistory(t *testing.T) {
	client := NewMockHelmClient()
	relKey := "test-ns/my-app"

	client.CurrentVersion[relKey] = 3
	client.Releases[relKey] = []*release.Release{
		{
			Name:      "my-app",
			Namespace: "test-ns",
			Version:   1,
			Info: &release.Info{
				Status: release.StatusDeployed,
			},
		},
		{
			Name:      "my-app",
			Namespace: "test-ns",
			Version:   2,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		},
		{
			Name:      "my-app",
			Namespace: "test-ns",
			Version:   3,
			Info: &release.Info{
				Status: release.StatusFailed,
			},
		},
	}

	// Intelligent selection of target: should skip current (3), skip failed (2), select deployed (1)
	target, err := client.GetLastHealthyRevision("test-ns", "my-app")
	if err != nil {
		t.Fatalf("unexpected error getting last healthy revision: %v", err)
	}

	if target != 1 {
		t.Errorf("expected target revision to be 1, got %d", target)
	}

	// Trigger Rollback to v1
	err = client.Rollback("test-ns", "my-app", 1)
	if err != nil {
		t.Fatalf("unexpected error rolling back: %v", err)
	}

	// Current revision should now be 4
	curr, err := client.GetCurrentRevision("test-ns", "my-app")
	if err != nil {
		t.Fatalf("unexpected error getting current version: %v", err)
	}

	if curr != 4 {
		t.Errorf("expected new current version to be 4, got %d", curr)
	}

	// History should now contain the 4th item
	history, err := client.GetHistory("test-ns", "my-app")
	if err != nil || len(history) != 4 {
		t.Errorf("expected history length 4, got %d", len(history))
	}
}
