// internal/operator/helm/manager.go
//
// Production-quality Helm client wrapping the Helm Go SDK.
// Avoids exec.Command shell invocations.
// Explains Helm SDK packages:
// - helm.sh/helm/v3/pkg/action: Handles commands/actions like list, status, history, rollback.
// - helm.sh/helm/v3/pkg/cli: Provides Helm CLI environment configurations and settings.
// - helm.sh/helm/v3/pkg/release: Models release entities (revision, status, chart info).
// - k8s.io/cli-runtime/pkg/genericclioptions: Bridges Kubernetes CLI config parameters to REST client commands.

package helm

import (
	"fmt"
	"os"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// HelmManager defines the contract for interfacing with Helm releases in Kubernetes.
type HelmManager interface {
	ListReleases(namespace string) ([]*release.Release, error)
	GetRelease(namespace, name string) (*release.Release, error)
	GetStatus(namespace, name string) (*release.Release, error)
	GetHistory(namespace, name string) ([]*release.Release, error)
	GetCurrentRevision(namespace, name string) (int, error)
	GetLastHealthyRevision(namespace, name string) (int, error)
	Rollback(namespace, name string, targetRevision int) error
}

type Client struct {
	settings *cli.EnvSettings
}

func NewClient() *Client {
	return &Client{
		settings: cli.New(),
	}
}

// getActionConfig initializes the action config required for executing SDK actions against a namespace.
func (c *Client) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)

	// Create Kubernetes client configuration flags using standard defaults.
	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.Namespace = &namespace

	// We use "secret" as the driver since it is the production standard for Helm release tracking in k8s.
	driver := os.Getenv("HELM_DRIVER")
	if driver == "" {
		driver = "secret"
	}

	// Init binds k8s API client configuration to actionConfig.
	err := actionConfig.Init(configFlags, namespace, driver, func(format string, v ...interface{}) {
		// Helm SDK logs can be muted or routed.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	return actionConfig, nil
}

// ListReleases returns all releases in the specified namespace.
func (c *Client) ListReleases(namespace string) ([]*release.Release, error) {
	cfg, err := c.getActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewList(cfg)
	client.All = true // show all releases (deployed, failed, superseded)

	releases, err := client.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list helm releases: %w", err)
	}
	return releases, nil
}

// GetRelease retrieves the current release by name and namespace.
func (c *Client) GetRelease(namespace, name string) (*release.Release, error) {
	cfg, err := c.getActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewGet(cfg)
	rel, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get helm release %q: %w", name, err)
	}
	return rel, nil
}

// GetStatus returns the current status (manifest, details) of a release.
func (c *Client) GetStatus(namespace, name string) (*release.Release, error) {
	// action.NewGet is also used to fetch release and inspect its Status
	return c.GetRelease(namespace, name)
}

// GetHistory returns all historical revisions for a release.
func (c *Client) GetHistory(namespace, name string) ([]*release.Release, error) {
	cfg, err := c.getActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	client := action.NewHistory(cfg)
	client.Max = 100 // limit to last 100 revisions

	history, err := client.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch helm release history for %q: %w", name, err)
	}
	return history, nil
}

// GetCurrentRevision returns the revision number of the current release.
func (c *Client) GetCurrentRevision(namespace, name string) (int, error) {
	rel, err := c.GetRelease(namespace, name)
	if err != nil {
		return 0, err
	}
	return rel.Version, nil
}

// GetLastHealthyRevision searches the history of a release backward to find the most recent deployed and healthy version.
func (c *Client) GetLastHealthyRevision(namespace, name string) (int, error) {
	history, err := c.GetHistory(namespace, name)
	if err != nil {
		return 0, err
	}

	currentRev, err := c.GetCurrentRevision(namespace, name)
	if err != nil {
		currentRev = 0
	}

	// History is sorted oldest to newest. We search backward from the end.
	for i := len(history) - 1; i >= 0; i-- {
		rel := history[i]

		// Skip the current version since we are rolling back to an older version.
		if rel.Version == currentRev {
			continue
		}

		// A release is a viable rollback target if its status is "deployed" (which means it was successfully configured).
		if rel.Info != nil && rel.Info.Status == release.StatusDeployed {
			return rel.Version, nil
		}
	}

	return 0, fmt.Errorf("no healthy historical revision found for release %q", name)
}

// Rollback triggers a Helm Rollback to a specific target revision.
func (c *Client) Rollback(namespace, name string, targetRevision int) error {
	cfg, err := c.getActionConfig(namespace)
	if err != nil {
		return err
	}

	client := action.NewRollback(cfg)
	client.Version = targetRevision
	client.Force = true // Recreate pods if needed
	client.Timeout = 0  // We handle recovery timeouts at the operator coordinator layer

	err = client.Run(name)
	if err != nil {
		return fmt.Errorf("helm rollback to revision %d failed: %w", targetRevision, err)
	}

	return nil
}
