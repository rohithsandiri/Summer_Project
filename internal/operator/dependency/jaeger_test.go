// internal/operator/dependency/jaeger_test.go

package dependency

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
)

// fakeJaegerClient lets tests control exactly what "observed traffic" the
// poller sees on each refresh, including simulating an unreachable Jaeger.
type fakeJaegerClient struct {
	mu       sync.Mutex
	edges    []Edge
	err      error
	callSeen int
}

func (f *fakeJaegerClient) GetDependencies(ctx context.Context, lookback time.Duration) ([]Edge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callSeen++
	if f.err != nil {
		return nil, f.err
	}
	return f.edges, nil
}

func (f *fakeJaegerClient) set(edges []Edge, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.edges = edges
	f.err = err
}

func TestJaegerGraphMergesObservedWithSeed(t *testing.T) {
	seed := NewEmptyGraph()
	seed.AddEdge("api-gateway", "order-service")

	client := &fakeJaegerClient{edges: []Edge{
		{Parent: "order-service", Child: "inventory-service", CallCount: 50},
		{Parent: "order-service", Child: "payment-service", CallCount: 30},
	}}

	g := NewJaegerDependencyGraph(client, seed, time.Minute, 1, logger.New("test"))

	// Seed edge preserved even though it wasn't in the observed set.
	if parents := g.GetParents("order-service"); len(parents) != 1 || parents[0] != "api-gateway" {
		t.Fatalf("expected seed edge api-gateway->order-service to survive merge, got parents=%v", parents)
	}
	// Newly observed edges present.
	children := g.GetChildren("order-service")
	if len(children) != 2 {
		t.Fatalf("expected 2 observed children of order-service, got %v", children)
	}
}

func TestJaegerGraphDiscoversNewServiceWithoutRestart(t *testing.T) {
	seed := NewEmptyGraph()
	seed.AddEdge("api-gateway", "order-service")
	client := &fakeJaegerClient{edges: nil} // nothing observed yet

	g := NewJaegerDependencyGraph(client, seed, time.Minute, 1, logger.New("test"))
	if children := g.GetChildren("order-service"); len(children) != 0 {
		t.Fatalf("expected no children before any traffic observed, got %v", children)
	}

	// A brand-new service (never in the hand-declared seed) starts calling
	// order-service — no code change, no restart, just a refresh cycle.
	client.set([]Edge{{Parent: "notification-service", Child: "order-service", CallCount: 5}}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx, 10*time.Millisecond)
	defer cancel()

	deadline := time.After(2 * time.Second)
	for {
		if parents := g.GetParents("order-service"); len(parents) == 2 {
			break // api-gateway (seed) + notification-service (newly observed)
		}
		select {
		case <-deadline:
			t.Fatalf("new service notification-service was never picked up; parents=%v", g.GetParents("order-service"))
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestJaegerGraphFallsBackToSeedWhenUnreachable(t *testing.T) {
	seed := NewGraph() // real hand-declared topology
	client := &fakeJaegerClient{err: context.DeadlineExceeded}

	g := NewJaegerDependencyGraph(client, seed, time.Minute, 1, logger.New("test"))

	// Construction's synchronous refresh failed; the coordinator must still
	// see the known-good seed topology, not an empty graph.
	if children := g.GetChildren("order-service"); len(children) != 2 {
		t.Fatalf("expected seed topology to remain available when Jaeger is unreachable, got %v", children)
	}
}

func TestJaegerGraphIgnoresLowCallCountNoise(t *testing.T) {
	seed := NewEmptyGraph()
	client := &fakeJaegerClient{edges: []Edge{
		{Parent: "order-service", Child: "debug-sidecar", CallCount: 1}, // stray single call
		{Parent: "order-service", Child: "inventory-service", CallCount: 200},
	}}

	g := NewJaegerDependencyGraph(client, seed, time.Minute, 10, logger.New("test"))

	children := g.GetChildren("order-service")
	for _, c := range children {
		if c == "debug-sidecar" {
			t.Fatalf("expected single stray call to be filtered by minCallCount threshold, got children=%v", children)
		}
	}
	if len(children) != 1 || children[0] != "inventory-service" {
		t.Fatalf("expected only inventory-service above threshold, got %v", children)
	}
}

func TestJaegerGraphSelfLoopIgnored(t *testing.T) {
	seed := NewEmptyGraph()
	client := &fakeJaegerClient{edges: []Edge{
		{Parent: "order-service", Child: "order-service", CallCount: 100}, // e.g. retry/self span artifact
	}}

	g := NewJaegerDependencyGraph(client, seed, time.Minute, 1, logger.New("test"))
	if children := g.GetChildren("order-service"); len(children) != 0 {
		t.Fatalf("expected self-loop edge to be dropped, got %v", children)
	}
}

func TestJaegerGraphConcurrentReadsDuringRefresh(t *testing.T) {
	seed := NewGraph()
	client := &fakeJaegerClient{edges: []Edge{{Parent: "order-service", Child: "inventory-service", CallCount: 5}}}
	g := NewJaegerDependencyGraph(client, seed, time.Minute, 1, logger.New("test"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go g.Run(ctx, time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = g.GetParents("order-service")
				_ = g.GetChildren("order-service")
				_ = g.GetDownstream("api-gateway")
			}
		}()
	}
	wg.Wait()
}
