// internal/operator/dependency/graph_test.go

package dependency

import (
	"testing"
)

func TestDependencyGraphTopology(t *testing.T) {
	g := NewGraph()

	// Verify standard edges:
	// api-gateway -> order-service -> payment-service
	// api-gateway -> order-service -> inventory-service

	parents := g.GetParents("order-service")
	if len(parents) != 1 || parents[0] != "api-gateway" {
		t.Errorf("expected api-gateway parent for order-service, got %v", parents)
	}

	children := g.GetChildren("order-service")
	if len(children) != 2 {
		t.Errorf("expected 2 children for order-service, got %d", len(children))
	}

	// Verify downstream of api-gateway recursively includes order, payment, and inventory services
	downstream := g.GetDownstream("api-gateway")
	if len(downstream) != 3 {
		t.Errorf("expected 3 downstream services, got %d", len(downstream))
	}

	hasOrder := false
	hasPayment := false
	hasInventory := false
	for _, d := range downstream {
		switch d {
		case "order-service":
			hasOrder = true
		case "payment-service":
			hasPayment = true
		case "inventory-service":
			hasInventory = true
		}
	}

	if !hasOrder || !hasPayment || !hasInventory {
		t.Errorf("missing expected downstream node: order=%v payment=%v inventory=%v", hasOrder, hasPayment, hasInventory)
	}

	// Verify upstream parents of payment-service recursively includes order and api-gateway
	upstream := g.GetUpstream("payment-service")
	if len(upstream) != 2 {
		t.Errorf("expected 2 upstream parents, got %d", len(upstream))
	}

	// Verify root services
	roots := g.GetRootServices()
	if len(roots) != 1 || roots[0] != "api-gateway" {
		t.Errorf("expected api-gateway as root, got %v", roots)
	}
}
