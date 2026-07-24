// internal/operator/dependency/coordinator_test.go

package dependency

import (
	"context"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/helm"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/utils"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
)

func rel(version int, deployed time.Time, status release.Status) *release.Release {
	return &release.Release{
		Version: version,
		Info: &release.Info{
			Status:       status,
			LastDeployed: helmtime.Time{Time: deployed},
		},
	}
}

// seed populates the mock with a release history for ns/name and marks the
// newest revision current.
func seed(m *helm.MockHelmClient, ns, name string, rels ...*release.Release) {
	key := ns + "/" + name
	m.Releases[key] = rels
	m.CurrentVersion[key] = rels[len(rels)-1].Version
}

func newTestCoordinator(m *helm.MockHelmClient) *Coordinator {
	return NewCoordinator(
		NewGraph(), // real topology: order-service → inventory-service, payment-service
		m,
		utils.NewCooldownManager(),
		logger.New("test"),
		10*time.Minute,
		5*time.Minute,
	)
}

func TestBatchPeerRolledBackWhenCoReleased(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	// inventory-service (the failed service): faulty rev 3 deployed now,
	// already rolled back by the primary pipeline (rev 4 current).
	seed(m, "default", "inventory-service",
		rel(2, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	// order-service (parent caller): upgraded 2 minutes before the faulty
	// revision — same release batch.
	seed(m, "default", "order-service",
		rel(1, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(2, now.Add(-2*time.Minute), release.StatusDeployed),
	)

	c := newTestCoordinator(m)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	var order *PeerResult
	for i := range results {
		if results[i].Service == "order-service" {
			order = &results[i]
		}
	}
	if order == nil {
		t.Fatal("order-service (direct parent) was not considered as a batch peer")
	}
	if !order.RolledBack {
		t.Fatalf("co-released parent should have been rolled back, got reason: %s", order.Reason)
	}
	if order.TargetRev != 1 {
		t.Fatalf("expected rollback target revision 1 (pre-batch), got %d", order.TargetRev)
	}
	if got := m.CurrentVersion["default/order-service"]; got == 2 {
		t.Fatal("mock current version should have advanced past 2 after rollback")
	}
}

func TestPeerOutsideBatchWindowLeftAlone(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	seed(m, "default", "inventory-service",
		rel(2, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	// order-service last deployed 3 hours ago — clearly not the same batch.
	seed(m, "default", "order-service",
		rel(1, now.Add(-4*time.Hour), release.StatusDeployed),
		rel(2, now.Add(-3*time.Hour), release.StatusDeployed),
	)

	c := newTestCoordinator(m)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	for _, r := range results {
		if r.Service == "order-service" && r.RolledBack {
			t.Fatalf("order-service deployed 3h before the faulty revision must not be rolled back (reason recorded: %s)", r.Reason)
		}
	}
	if got := m.CurrentVersion["default/order-service"]; got != 2 {
		t.Fatalf("order-service revision should be untouched at 2, got %d", got)
	}
}

func TestPeerSkippedWhenGateAlreadyClaimed(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	seed(m, "default", "inventory-service",
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	seed(m, "default", "order-service",
		rel(1, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(2, now.Add(-1*time.Minute), release.StatusDeployed),
	)

	gate := utils.NewCooldownManager()
	// order-service already recovered via its own alert-driven pipeline.
	gate.TryStartExecution(context.Background(), "order-service", 5*time.Minute)

	c := NewCoordinator(NewGraph(), m, gate, logger.New("test"), 10*time.Minute, 5*time.Minute)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	for _, r := range results {
		if r.Service == "order-service" && r.RolledBack {
			t.Fatal("peer with a claimed execution gate must not be rolled back again")
		}
	}
	if got := m.CurrentVersion["default/order-service"]; got != 2 {
		t.Fatalf("order-service revision should be untouched at 2, got %d", got)
	}
}

// fakeVerifier is a configurable PodVerifier for testing the coordinator's
// post-rollback health check without a real Kubernetes clientset.
type fakeVerifier struct {
	healthy bool
	calls   int
}

func (f *fakeVerifier) Verify(ctx context.Context, service, namespace, traceID string) (bool, error) {
	f.calls++
	return f.healthy, nil
}

// TestPeerGateNotClaimedWhenNoHealthyTarget guards against a real bug found
// while reviewing the coordinator: it used to claim the shared per-service
// execution gate BEFORE calling GetLastHealthyRevision, so a peer with no
// healthy history entry (e.g. every prior revision Superseded/Failed) burned
// its execution slot for a full gateWindow despite no rollback ever being
// attempted — incorrectly suppressing that service's own next legitimate
// alert-driven recovery. The gate must only be claimed once a target has
// actually been resolved and a rollback is about to be attempted.
func TestPeerGateNotClaimedWhenNoHealthyTarget(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	seed(m, "default", "inventory-service",
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	// order-service: current revision 2, but revision 1 is Superseded (not
	// Deployed), so GetLastHealthyRevision has no valid target.
	seed(m, "default", "order-service",
		rel(1, now.Add(-2*time.Minute), release.StatusSuperseded),
		rel(2, now.Add(-1*time.Minute), release.StatusDeployed),
	)

	gate := utils.NewCooldownManager()
	c := NewCoordinator(NewGraph(), m, gate, logger.New("test"), 10*time.Minute, 5*time.Minute)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	for _, r := range results {
		if r.Service == "order-service" && r.RolledBack {
			t.Fatal("order-service has no healthy target and must not be reported as rolled back")
		}
	}

	// The real assertion: the gate must still be free, since no rollback was
	// ever attempted for order-service.
	if ok, remaining := gate.TryStartExecution(context.Background(), "order-service", 5*time.Minute); !ok {
		t.Fatalf("execution gate for order-service was incorrectly claimed by the failed target lookup (remaining: %s)", remaining)
	}
}

// TestPeerRolledBackButUnverified confirms that when a verifier is attached,
// a peer rollback which never comes back healthy is reported with
// RolledBack=true, Verified=false — not a silent success — mirroring the
// SLO-aware verification the primary rollback path already applies (§6.4).
func TestPeerRolledBackButUnverified(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	seed(m, "default", "inventory-service",
		rel(2, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	seed(m, "default", "order-service",
		rel(1, now.Add(-1*time.Hour), release.StatusDeployed),
		rel(2, now.Add(-2*time.Minute), release.StatusDeployed),
	)

	verifier := &fakeVerifier{healthy: false}
	c := newTestCoordinator(m).WithVerifier(verifier, 50*time.Millisecond)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	var order *PeerResult
	for i := range results {
		if results[i].Service == "order-service" {
			order = &results[i]
		}
	}
	if order == nil {
		t.Fatal("order-service was not considered as a batch peer")
	}
	if !order.RolledBack {
		t.Fatalf("Helm rollback call did succeed and should be reported as issued, reason: %s", order.Reason)
	}
	if order.Verified {
		t.Fatal("verifier reported unhealthy; PeerResult.Verified must be false")
	}
	if verifier.calls == 0 {
		t.Fatal("verifier was never invoked")
	}
}

func TestPeerOnInitialRevisionSkipped(t *testing.T) {
	now := time.Now()
	m := helm.NewMockHelmClient()

	seed(m, "default", "inventory-service",
		rel(3, now, release.StatusSuperseded),
		rel(4, now.Add(30*time.Second), release.StatusDeployed),
	)
	// order-service has only its initial install — nothing to roll back to.
	seed(m, "default", "order-service",
		rel(1, now.Add(-1*time.Minute), release.StatusDeployed),
	)

	c := newTestCoordinator(m)
	results := c.RollbackBatchPeers(context.Background(), "inventory-service", "default", 3, "tr-test")

	for _, r := range results {
		if r.Service == "order-service" && r.RolledBack {
			t.Fatal("peer on its initial revision must not be rolled back")
		}
	}
}
