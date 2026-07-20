// internal/operator/dependency/jaeger.go
//
// Self-maintaining dependency graph, derived from real observed service
// calls instead of a hand-maintained edge list.
//
// Approach (trace-based dependency-graph construction — the same technique
// production RCA systems like Microscope and MicroHECL use to build their
// call graphs from distributed traces): Jaeger's query service already
// aggregates every span's caller/callee relationship into a dependency
// endpoint (GET /api/dependencies?endTs&lookback → [(parent, child, callCount), ...]).
// This type polls that endpoint on an interval and swaps in a fresh graph,
// so a new service or a new call path is picked up automatically the next
// time it actually makes a request — no code change, no rebuild, no restart.
//
// Cold-start / quiet-traffic safety net: the poller merges observed edges
// with a static seed graph (typically dependency.NewGraph()'s hand-declared
// topology) rather than replacing it outright. A known contract relationship
// must not vanish from the graph just because no call happened to occur in
// the last lookback window — that would silently disable batch rollback for
// it at the worst possible time (during an incident, when the coordinator
// needs it).

package dependency

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
)

// Edge is one observed or declared caller→callee relationship.
type Edge struct {
	Parent    string
	Child     string
	CallCount int
}

// JaegerClient fetches observed service dependencies. Its own interface
// (rather than depending on the Jaeger SDK) keeps this package's only
// external dependency an HTTP call, and makes the poller trivially testable
// with a fake.
type JaegerClient interface {
	GetDependencies(ctx context.Context, lookback time.Duration) ([]Edge, error)
}

// httpJaegerClient implements JaegerClient against Jaeger's query-service
// REST API, i.e. the same all-in-one deployment k8s/monitoring/jaeger.yaml
// installs.
type httpJaegerClient struct {
	baseURL string
	hc      *http.Client
}

func NewHTTPJaegerClient(baseURL string) JaegerClient {
	return &httpJaegerClient{
		baseURL: baseURL,
		hc:      &http.Client{Timeout: 10 * time.Second},
	}
}

type jaegerDependenciesResponse struct {
	Data []struct {
		Parent    string `json:"parent"`
		Child     string `json:"child"`
		CallCount int    `json:"callCount"`
	} `json:"data"`
	Errors []struct {
		Msg string `json:"msg"`
	} `json:"errors"`
}

func (c *httpJaegerClient) GetDependencies(ctx context.Context, lookback time.Duration) ([]Edge, error) {
	endTs := time.Now().UnixMilli()
	url := fmt.Sprintf("%s/api/dependencies?endTs=%d&lookback=%d", c.baseURL, endTs, lookback.Milliseconds())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query jaeger dependencies: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read jaeger response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger dependencies returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed jaegerDependenciesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse jaeger response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("jaeger returned errors: %s", parsed.Errors[0].Msg)
	}

	edges := make([]Edge, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		edges = append(edges, Edge{Parent: d.Parent, Child: d.Child, CallCount: d.CallCount})
	}
	return edges, nil
}

// JaegerDependencyGraph implements DependencyGraph, backed by a periodically
// refreshed snapshot merged from observed Jaeger edges and a static seed.
type JaegerDependencyGraph struct {
	client       JaegerClient
	seed         *Graph
	lookback     time.Duration
	minCallCount int
	log          *logger.Logger

	mu      sync.RWMutex
	current *Graph
}

// NewJaegerDependencyGraph builds the graph and runs one synchronous refresh
// before returning, so the very first coordinator decision already has the
// best available data (observed if Jaeger answered in time, seed-only
// otherwise) instead of an empty graph.
func NewJaegerDependencyGraph(
	client JaegerClient,
	seed *Graph,
	lookback time.Duration,
	minCallCount int,
	log *logger.Logger,
) *JaegerDependencyGraph {
	if minCallCount < 1 {
		minCallCount = 1
	}
	jdg := &JaegerDependencyGraph{
		client:       client,
		seed:         seed,
		lookback:     lookback,
		minCallCount: minCallCount,
		log:          log,
		current:      seed,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	jdg.refresh(ctx)

	return jdg
}

// Run starts the background refresh loop; it blocks until ctx is cancelled,
// so callers should invoke it with `go jdg.Run(ctx, interval)`.
func (jdg *JaegerDependencyGraph) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jdg.refresh(ctx)
		}
	}
}

func (jdg *JaegerDependencyGraph) refresh(ctx context.Context) {
	observed, err := jdg.client.GetDependencies(ctx, jdg.lookback)
	if err != nil {
		jdg.log.Warn(ctx, "dependency graph refresh failed, keeping previous snapshot", logger.Fields{}, "error", err.Error())
		return
	}

	merged := NewEmptyGraph()
	for _, e := range jdg.seed.Edges() {
		merged.AddEdge(e[0], e[1])
	}
	skippedNoise := 0
	for _, e := range observed {
		if e.Parent == "" || e.Child == "" || e.Parent == e.Child {
			continue
		}
		if e.CallCount < jdg.minCallCount {
			skippedNoise++
			continue
		}
		merged.AddEdge(e.Parent, e.Child)
	}

	added, removed := diffEdges(jdg.snapshot().Edges(), merged.Edges())

	jdg.mu.Lock()
	jdg.current = merged
	jdg.mu.Unlock()

	if len(added) > 0 || len(removed) > 0 {
		jdg.log.Info(ctx, "dependency graph refreshed from observed traffic", logger.Fields{},
			"edges_added", fmt.Sprintf("%v", added),
			"edges_removed", fmt.Sprintf("%v", removed),
			"observed_edges", len(observed),
			"skipped_below_threshold", skippedNoise,
		)
	}
}

func diffEdges(before, after [][2]string) (added, removed [][2]string) {
	beforeSet := make(map[[2]string]bool, len(before))
	for _, e := range before {
		beforeSet[e] = true
	}
	afterSet := make(map[[2]string]bool, len(after))
	for _, e := range after {
		afterSet[e] = true
	}
	for e := range afterSet {
		if !beforeSet[e] {
			added = append(added, e)
		}
	}
	for e := range beforeSet {
		if !afterSet[e] {
			removed = append(removed, e)
		}
	}
	return added, removed
}

func (jdg *JaegerDependencyGraph) snapshot() *Graph {
	jdg.mu.RLock()
	defer jdg.mu.RUnlock()
	return jdg.current
}

func (jdg *JaegerDependencyGraph) GetParents(service string) []string {
	return jdg.snapshot().GetParents(service)
}
func (jdg *JaegerDependencyGraph) GetChildren(service string) []string {
	return jdg.snapshot().GetChildren(service)
}
func (jdg *JaegerDependencyGraph) GetUpstream(service string) []string {
	return jdg.snapshot().GetUpstream(service)
}
func (jdg *JaegerDependencyGraph) GetDownstream(service string) []string {
	return jdg.snapshot().GetDownstream(service)
}
func (jdg *JaegerDependencyGraph) GetRootServices() []string { return jdg.snapshot().GetRootServices() }
