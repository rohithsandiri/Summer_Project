// internal/operator/dependency/graph.go
//
// Dependency Graph Manager. Traces service interactions (upstream, downstream, parent, child).

package dependency

type Node struct {
	ID       string
	Parents  []*Node
	Children []*Node
}

type DependencyGraph interface {
	GetParents(service string) []string
	GetChildren(service string) []string
	GetUpstream(service string) []string   // Recursive parents
	GetDownstream(service string) []string // Recursive children
	GetRootServices() []string
}

type Graph struct {
	nodes map[string]*Node
}

func NewGraph() *Graph {
	g := NewEmptyGraph()

	// Pre-populate system topology as specified in phase specification.
	// This hand-declared graph is also used as the safety-net seed for
	// JaegerDependencyGraph (see jaeger.go): known contract relationships
	// that must never disappear just because traffic was briefly quiet.
	g.AddEdge("api-gateway", "order-service")
	g.AddEdge("order-service", "inventory-service")
	g.AddEdge("order-service", "payment-service")

	return g
}

// NewEmptyGraph returns a graph with no pre-populated edges, for callers
// (such as trace-derived graph builders) that construct topology themselves.
func NewEmptyGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

// NewGeneratedSeedGraph builds a Graph from GeneratedSeedEdges
// (generated_seed.go, produced by tools/depscan statically scanning
// cmd/*/main.go for `<Name>ServiceURL` config-field references). This is
// what production wiring (cmd/operator/main.go) uses as the safety-net floor
// for JaegerDependencyGraph, in place of the hand-maintained NewGraph():
// a new caller/callee relationship added in code is picked up the next time
// `make depscan` runs (enforced by a CI drift check), instead of silently
// not participating in batch rollback until someone remembers to hand-edit
// the seed. NewGraph() is kept as-is — a fixed, deliberately-scoped fixture
// several tests assert exact structure against — and is not used in
// production anymore.
func NewGeneratedSeedGraph() *Graph {
	g := NewEmptyGraph()
	for _, e := range GeneratedSeedEdges {
		g.AddEdge(e[0], e[1])
	}
	return g
}

// Edges returns every (parent, child) pair currently in the graph. Order is
// unspecified. Used for merging and diffing graph snapshots.
func (g *Graph) Edges() [][2]string {
	var edges [][2]string
	for _, n := range g.nodes {
		for _, c := range n.Children {
			edges = append(edges, [2]string{n.ID, c.ID})
		}
	}
	return edges
}

func (g *Graph) ensureNode(id string) *Node {
	if n, ok := g.nodes[id]; ok {
		return n
	}
	n := &Node{ID: id}
	g.nodes[id] = n
	return n
}

func (g *Graph) AddEdge(parentID, childID string) {
	parent := g.ensureNode(parentID)
	child := g.ensureNode(childID)

	// Check if already registered
	for _, c := range parent.Children {
		if c.ID == childID {
			return
		}
	}

	parent.Children = append(parent.Children, child)
	child.Parents = append(child.Parents, parent)
}

func (g *Graph) GetParents(service string) []string {
	n, ok := g.nodes[service]
	if !ok {
		return nil
	}
	res := make([]string, 0, len(n.Parents))
	for _, p := range n.Parents {
		res = append(res, p.ID)
	}
	return res
}

func (g *Graph) GetChildren(service string) []string {
	n, ok := g.nodes[service]
	if !ok {
		return nil
	}
	res := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		res = append(res, c.ID)
	}
	return res
}

func (g *Graph) GetUpstream(service string) []string {
	visited := make(map[string]bool)
	g.dfsParents(service, visited)

	// Remove self
	delete(visited, service)

	res := make([]string, 0, len(visited))
	for k := range visited {
		res = append(res, k)
	}
	return res
}

func (g *Graph) dfsParents(id string, visited map[string]bool) {
	if visited[id] {
		return
	}
	visited[id] = true
	n, ok := g.nodes[id]
	if !ok {
		return
	}
	for _, p := range n.Parents {
		g.dfsParents(p.ID, visited)
	}
}

func (g *Graph) GetDownstream(service string) []string {
	visited := make(map[string]bool)
	g.dfsChildren(service, visited)

	// Remove self
	delete(visited, service)

	res := make([]string, 0, len(visited))
	for k := range visited {
		res = append(res, k)
	}
	return res
}

func (g *Graph) dfsChildren(id string, visited map[string]bool) {
	if visited[id] {
		return
	}
	visited[id] = true
	n, ok := g.nodes[id]
	if !ok {
		return
	}
	for _, c := range n.Children {
		g.dfsChildren(c.ID, visited)
	}
}

func (g *Graph) GetRootServices() []string {
	var roots []string
	for id, n := range g.nodes {
		if len(n.Parents) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}
