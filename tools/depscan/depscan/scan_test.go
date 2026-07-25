// tools/depscan/depscan/scan_test.go

package depscan

import (
	"os"
	"strings"
	"testing"
)

func hasEdge(edges []Edge, caller, callee string) bool {
	for _, e := range edges {
		if e.Caller == caller && e.Callee == callee {
			return true
		}
	}
	return false
}

func TestScanFileFindsHttpclientNewPattern(t *testing.T) {
	src := []byte(`package main

import "github.com/rohithsandiri/Summer_Project/internal/shared/httpclient"

func main() {
	cfg, _ := config.Load("order-service", config.LoadOptions{})
	inventoryClient := httpclient.New(cfg.InventoryServiceURL, "inventory-service")
	paymentClient := httpclient.New(cfg.PaymentServiceURL, "payment-service")
	_ = inventoryClient
	_ = paymentClient
}
`)
	caller, edges, err := ScanFile("order-service/main.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller != "order-service" {
		t.Fatalf("caller = %q, want order-service", caller)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d: %v", len(edges), edges)
	}
	if !hasEdge(edges, "order-service", "inventory-service") {
		t.Errorf("missing edge order-service -> inventory-service, got %v", edges)
	}
	if !hasEdge(edges, "order-service", "payment-service") {
		t.Errorf("missing edge order-service -> payment-service, got %v", edges)
	}
}

func TestScanFileFindsRouteConfigPattern(t *testing.T) {
	// The gateway doesn't call httpclient.New at all — it builds
	// handler.RouteConfig{BackendURL: cfg.XServiceURL} reverse-proxy routes.
	// The scanner must catch this too, since both shapes go through the same
	// named config field.
	src := []byte(`package main

func main() {
	cfg, _ := config.Load("api-gateway", config.LoadOptions{})
	routes := []handler.RouteConfig{
		{Prefix: "/api/orders", BackendURL: cfg.OrderServiceURL},
		{Prefix: "/api/inventory", BackendURL: cfg.InventoryServiceURL},
		{Prefix: "/api/payments", BackendURL: cfg.PaymentServiceURL},
	}
	_ = routes
}
`)
	caller, edges, err := ScanFile("gateway/main.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller != "api-gateway" {
		t.Fatalf("caller = %q, want api-gateway", caller)
	}
	want := []string{"order-service", "inventory-service", "payment-service"}
	if len(edges) != len(want) {
		t.Fatalf("expected %d edges, got %d: %v", len(want), len(edges), edges)
	}
	for _, callee := range want {
		if !hasEdge(edges, "api-gateway", callee) {
			t.Errorf("missing edge api-gateway -> %s, got %v", callee, edges)
		}
	}
}

func TestScanFileSkipsFilesWithNoConfigLoadLiteral(t *testing.T) {
	// cmd/operator/main.go calls config.Load() with no arguments; cmd/demo
	// and cmd/healthcheck don't call it at all. None of these participate in
	// the caller/callee convention, and that must not be treated as an error.
	src := []byte(`package main

func main() {
	cfg, _ := config.Load()
	_ = cfg
}
`)
	caller, edges, err := ScanFile("operator/main.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller != "" {
		t.Fatalf("expected no caller resolved, got %q", caller)
	}
	if edges != nil {
		t.Fatalf("expected no edges, got %v", edges)
	}
}

func TestScanFileIgnoresSelfReference(t *testing.T) {
	// A service referencing a config field that happens to name itself must
	// not produce a self-loop edge.
	src := []byte(`package main

func main() {
	cfg, _ := config.Load("order-service", config.LoadOptions{})
	_ = cfg.OrderServiceURL
	_ = cfg.InventoryServiceURL
}
`)
	caller, edges, err := ScanFile("order-service/main.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller != "order-service" {
		t.Fatalf("caller = %q, want order-service", caller)
	}
	if len(edges) != 1 || edges[0].Callee != "inventory-service" {
		t.Fatalf("expected only the inventory-service edge (self-loop dropped), got %v", edges)
	}
}

func TestScanFileRejectsInvalidGo(t *testing.T) {
	_, _, err := ScanFile("broken/main.go", []byte("this is not valid go source {{{"))
	if err == nil {
		t.Fatal("expected a parse error for invalid Go source")
	}
}

func TestKebabFromPascalHandlesMultiWordNames(t *testing.T) {
	cases := map[string]string{
		"Order":           "order",
		"Inventory":       "inventory",
		"OrderProcessing": "order-processing",
		"UserProfileAPI":  "user-profile-api",
	}
	for in, want := range cases {
		if got := kebabFromPascal(in); got != want {
			t.Errorf("kebabFromPascal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanRootDedupesAcrossFiles(t *testing.T) {
	root := t.TempDir()
	writeMain := func(dir, src string) {
		full := root + "/cmd/" + dir
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full+"/main.go", []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeMain("order-service", `package main
func main() {
	cfg, _ := config.Load("order-service", config.LoadOptions{})
	_ = cfg.InventoryServiceURL
	_ = cfg.PaymentServiceURL
}`)
	writeMain("gateway", `package main
func main() {
	cfg, _ := config.Load("api-gateway", config.LoadOptions{})
	_ = cfg.OrderServiceURL
}`)
	writeMain("demo", `package main
func main() {}`)

	edges, err := ScanRoot(root)
	if err != nil {
		t.Fatalf("ScanRoot error: %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d: %v", len(edges), edges)
	}
	if !hasEdge(edges, "api-gateway", "order-service") {
		t.Errorf("missing api-gateway -> order-service, got %v", edges)
	}
	if !hasEdge(edges, "order-service", "inventory-service") {
		t.Errorf("missing order-service -> inventory-service, got %v", edges)
	}
	if !hasEdge(edges, "order-service", "payment-service") {
		t.Errorf("missing order-service -> payment-service, got %v", edges)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	edges := []Edge{
		{Caller: "order-service", Callee: "payment-service"},
		{Caller: "api-gateway", Callee: "order-service"},
		{Caller: "order-service", Callee: "inventory-service"},
	}
	out := string(Generate(edges))
	if !strings.Contains(out, `{"api-gateway", "order-service"},`) {
		t.Errorf("generated output missing expected edge line:\n%s", out)
	}
	// Caller-then-callee sort must put api-gateway's edge before order-service's.
	gatewayIdx := strings.Index(out, `"api-gateway"`)
	orderIdx := strings.Index(out, `"order-service", "inventory-service"`)
	if gatewayIdx == -1 || orderIdx == -1 || gatewayIdx > orderIdx {
		t.Errorf("expected edges sorted by caller then callee, got:\n%s", out)
	}
	if !strings.HasPrefix(out, "// Code generated by tools/depscan") {
		t.Errorf("expected generated-code header, got:\n%s", out)
	}
}
