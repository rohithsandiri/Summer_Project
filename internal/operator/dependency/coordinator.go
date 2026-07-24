// internal/operator/dependency/coordinator.go
//
// Batch-aware dependency rollback coordinator.
//
// Problem: services are often released together as a batch — a callee ships a
// new API endpoint and its callers, upgraded in the same batch, start calling
// it. If the callee later fails and is rolled back alone, the still-upgraded
// callers now call an endpoint that no longer exists, turning one incident
// into a cascade of contract-mismatch failures. The safe unit of rollback is
// the batch, not the single service.
//
// After the primary service's rollback succeeds, the coordinator consults the
// dependency graph for tightly-coupled services (direct parents and children)
// and rolls back any whose current revision was deployed within the batch
// window of the primary's faulty revision — the signature of "released
// together". Each peer rollback claims the same per-service execution gate as
// alert-driven recoveries, so a peer that was already rolled back by its own
// incident (or by an earlier batch expansion) is skipped, and expansion never
// recurses: only peers of the alert-triggered primary are considered.

package dependency

import (
	"context"
	"fmt"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"helm.sh/helm/v3/pkg/release"
)

// HelmOps is the subset of the Helm manager the coordinator needs.
type HelmOps interface {
	GetHistory(namespace, name string) ([]*release.Release, error)
	GetCurrentRevision(namespace, name string) (int, error)
	GetLastHealthyRevision(namespace, name string) (int, error)
	Rollback(namespace, name string, targetRevision int) error
}

// PodVerifier confirms a service is actually healthy after a rollback. Its
// method matches verification.VerificationEngine's signature structurally
// (Go interfaces are satisfied implicitly), so the operator's real
// K8sVerificationEngine can be handed to the coordinator with no import
// coupling between the two packages, and fakes are trivial in tests.
type PodVerifier interface {
	Verify(ctx context.Context, service string, namespace string, traceID string) (bool, error)
}

// PeerResult records the outcome for one candidate peer service.
type PeerResult struct {
	Service string
	// RolledBack reports whether a Helm rollback call was issued and
	// returned success. It does NOT by itself mean the peer came back
	// healthy — check Verified for that.
	RolledBack bool
	// Verified reports whether the peer was confirmed healthy after the
	// rollback. Always true when RolledBack is false (nothing to verify)
	// and always true when no verifier is configured, preserving the
	// coordinator's original pod-health-blind behavior.
	Verified  bool
	TargetRev int
	Reason    string
}

type Coordinator struct {
	graph         DependencyGraph
	helm          HelmOps
	gate          interfaces.CooldownManager
	log           *logger.Logger
	batchWindow   time.Duration
	gateWindow    time.Duration
	verifier      PodVerifier
	verifyTimeout time.Duration
}

func NewCoordinator(
	graph DependencyGraph,
	helm HelmOps,
	gate interfaces.CooldownManager,
	log *logger.Logger,
	batchWindow time.Duration,
	gateWindow time.Duration,
) *Coordinator {
	if batchWindow <= 0 {
		batchWindow = 10 * time.Minute
	}
	if gateWindow <= 0 {
		gateWindow = 5 * time.Minute
	}
	return &Coordinator{
		graph:       graph,
		helm:        helm,
		gate:        gate,
		log:         log,
		batchWindow: batchWindow,
		gateWindow:  gateWindow,
	}
}

// WithVerifier enables post-rollback health verification for batch peers,
// extending to peers the same "reported success must be verified" discipline
// the primary alert-driven rollback already follows (verification.SLOVerifier,
// added after the live-failure suite found rollbacks were reporting success
// while still serving faults — F1/§6.4). Without it, a peer rollback is
// reported RolledBack the instant the Helm SDK call returns, even if the
// target revision never comes up healthy.
func (c *Coordinator) WithVerifier(v PodVerifier, timeout time.Duration) *Coordinator {
	c.verifier = v
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	c.verifyTimeout = timeout
	return c
}

// RollbackBatchPeers rolls back services tightly coupled to failedService that
// were deployed in the same batch as its faulty revision. faultyRevision is
// the revision the primary rollback just moved away from (the executor's
// OldRevision); its deploy time anchors the batch window.
func (c *Coordinator) RollbackBatchPeers(
	ctx context.Context,
	failedService string,
	namespace string,
	faultyRevision int,
	traceID string,
) []PeerResult {
	f := logger.Fields{
		TraceID: traceID,
		Service: failedService,
	}

	batchRef, err := c.deployTime(namespace, failedService, faultyRevision)
	if err != nil {
		c.log.Warn(ctx, "dependency rollback skipped: cannot determine faulty revision deploy time", f, "error", err.Error())
		return nil
	}

	// Direct parents (callers) and children (callees): the services whose API
	// contracts the failed service participates in. Transitive closure is
	// deliberately excluded — coupling weakens with distance, and expanding
	// the blast radius of a rollback beyond direct contract partners does
	// more harm than good.
	peers := append(c.graph.GetParents(failedService), c.graph.GetChildren(failedService)...)

	var results []PeerResult
	for _, peer := range peers {
		results = append(results, c.considerPeer(ctx, peer, namespace, batchRef, traceID))
	}
	return results
}

func (c *Coordinator) considerPeer(ctx context.Context, peer, namespace string, batchRef time.Time, traceID string) PeerResult {
	f := logger.Fields{
		TraceID: traceID,
		Service: peer,
	}

	currentRev, err := c.helm.GetCurrentRevision(namespace, peer)
	if err != nil {
		return PeerResult{Service: peer, Reason: fmt.Sprintf("no helm release found: %v", err)}
	}
	if currentRev <= 1 {
		return PeerResult{Service: peer, Reason: "initial revision, nothing to roll back to"}
	}

	peerDeployTime, err := c.deployTime(namespace, peer, currentRev)
	if err != nil {
		return PeerResult{Service: peer, Reason: fmt.Sprintf("cannot determine deploy time: %v", err)}
	}

	gap := peerDeployTime.Sub(batchRef)
	if gap < 0 {
		gap = -gap
	}
	if gap > c.batchWindow {
		c.log.Info(ctx, "dependency peer not part of deployment batch, leaving as-is", f,
			"deploy_gap", gap.String(), "batch_window", c.batchWindow.String())
		return PeerResult{Service: peer, Reason: fmt.Sprintf("deployed %s from batch anchor (window %s) — not co-released", gap.Round(time.Second), c.batchWindow)}
	}

	// Resolve the rollback target BEFORE claiming the execution gate. The
	// gate is a shared resource with the alert-driven pipeline (utils.
	// CooldownManager.TryStartExecution) — claiming it here and then bailing
	// out below (no healthy target found) would burn the peer's execution
	// slot for a full gateWindow without any rollback ever happening,
	// incorrectly suppressing a legitimate later recovery for that same
	// service. Only claim the gate once we're actually about to execute.
	targetRev, err := c.helm.GetLastHealthyRevision(namespace, peer)
	if err != nil {
		return PeerResult{Service: peer, Verified: true, Reason: fmt.Sprintf("no healthy rollback target: %v", err)}
	}

	// Same gate the alert-driven pipeline uses: if this peer was already
	// rolled back (own incident, or another batch expansion), skip it.
	if ok, remaining := c.gate.TryStartExecution(ctx, peer, c.gateWindow); !ok {
		c.log.Info(ctx, "dependency peer already recovered recently, skipping batch rollback", f, "remaining_window", remaining.String())
		return PeerResult{Service: peer, Verified: true, Reason: fmt.Sprintf("recovery already ran %s ago", remaining.Round(time.Second))}
	}

	c.log.Info(ctx, "rolling back dependency batch peer", f,
		"current_revision", currentRev, "target_revision", targetRev, "deploy_gap", gap.String())

	if err := c.helm.Rollback(namespace, peer, targetRev); err != nil {
		c.log.Error(ctx, "dependency batch peer rollback failed", f, "error", err.Error())
		return PeerResult{Service: peer, Verified: true, Reason: fmt.Sprintf("rollback failed: %v", err)}
	}

	verified, reason := c.verifyPeer(ctx, peer, namespace, traceID)
	if !verified {
		c.log.Warn(ctx, "dependency batch peer rolled back but did not verify healthy", f,
			"target_revision", targetRev, "reason", reason)
		return PeerResult{
			Service: peer, RolledBack: true, Verified: false, TargetRev: targetRev,
			Reason: fmt.Sprintf("rolled back to revision %d but did not verify healthy within %s: %s — needs investigation", targetRev, c.verifyTimeout, reason),
		}
	}

	return PeerResult{Service: peer, RolledBack: true, Verified: true, TargetRev: targetRev, Reason: "co-released within batch window"}
}

// verifyPeer polls the configured verifier until it reports healthy or
// verifyTimeout elapses. With no verifier configured it returns true
// immediately (pod-health-blind mode, matching pre-verification behavior).
func (c *Coordinator) verifyPeer(ctx context.Context, peer, namespace, traceID string) (bool, string) {
	if c.verifier == nil {
		return true, ""
	}

	deadline := time.Now().Add(c.verifyTimeout)
	var lastErr error
	for {
		ok, err := c.verifier.Verify(ctx, peer, namespace, traceID)
		if err != nil {
			lastErr = err
		} else if ok {
			return true, ""
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return false, lastErr.Error()
			}
			return false, "pod health check did not pass in time"
		}
		select {
		case <-ctx.Done():
			return false, "context cancelled"
		case <-time.After(2 * time.Second):
		}
	}
}

func (c *Coordinator) deployTime(namespace, service string, revision int) (time.Time, error) {
	history, err := c.helm.GetHistory(namespace, service)
	if err != nil {
		return time.Time{}, err
	}
	for _, rel := range history {
		if rel.Version == revision && rel.Info != nil {
			return rel.Info.LastDeployed.Time, nil
		}
	}
	return time.Time{}, fmt.Errorf("revision %d not found in history of %s", revision, service)
}
