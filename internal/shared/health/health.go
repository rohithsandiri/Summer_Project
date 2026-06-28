// internal/shared/health/health.go
//
// WHY THIS EXISTS:
//   Kubernetes uses two probes to manage pod lifecycle:
//
//   LIVENESS PROBE  → GET /health/live
//     "Is the process alive and not deadlocked?"
//     If this returns non-200, Kubernetes RESTARTS the pod.
//     Must be fast (<1s) and must NOT check external dependencies.
//     A liveness probe that pings the database would cause cascading restarts
//     when the database is slow — killing healthy pods unnecessarily.
//
//   READINESS PROBE → GET /health/ready
//     "Is the process ready to receive traffic?"
//     If this returns non-200, Kubernetes removes the pod from the Service
//     load balancer but does NOT restart it.
//     This is where we check external dependencies (DB connectivity, etc.)
//     When a downstream service is slow, the pod becomes "not ready" and
//     traffic stops flowing to it — preventing cascading failures.
//
//   STARTUP PROBE   → GET /health/startup
//     Used during initial pod startup. Kubernetes waits for this to succeed
//     before starting liveness/readiness probes. Prevents premature restarts
//     of slow-starting services.
//
// PHASE 1 SIMPLIFICATION:
//   With in-memory repositories there are no external dependencies to check.
//   We still implement the full three-probe pattern so Kubernetes manifests in
//   Phase 3 can be configured immediately without changing service code.
//
// RESPONSE FORMAT:
//   {
//     "status": "healthy" | "degraded" | "unhealthy",
//     "service": "order-service",
//     "version": "1.0.0",
//     "uptime_seconds": 3600,
//     "started_at": "2026-06-27T08:00:00Z",
//     "checks": {
//       "repository": { "status": "ok", "message": "in-memory" },
//       "inventory_service": { "status": "ok", "message": "configured" }
//     }
//   }
//
// FUTURE PHASES:
//   - Add real connectivity check: ping PostgreSQL, check Redis PING
//   - Add dependency circuit-breaker state in the checks map
//   - Report memory usage and goroutine count as saturation signals

package health

import (
	"net/http"
	"sync"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/response"
)

// Status represents the health state of a component.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// CheckResult is the health status of a single dependency or component.
type CheckResult struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

// Response is the full health check response body.
type Response struct {
	Status        Status                 `json:"status"`
	Service       string                 `json:"service"`
	Version       string                 `json:"version"`
	UptimeSeconds float64                `json:"uptime_seconds"`
	StartedAt     string                 `json:"started_at"`
	Checks        map[string]CheckResult `json:"checks"`
}

// CheckFunc is a function that performs a single health check.
// Returns CheckResult — callers must not panic.
type CheckFunc func() CheckResult

// Handler provides three health endpoints for a service.
// It is constructed once at startup and registered on the mux.
type Handler struct {
	serviceName string
	version     string
	startedAt   time.Time

	mu     sync.RWMutex
	checks map[string]CheckFunc // named dependency checks
}

// NewHandler creates a health Handler for a service.
func NewHandler(serviceName, version string) *Handler {
	return &Handler{
		serviceName: serviceName,
		version:     version,
		startedAt:   time.Now().UTC(),
		checks:      make(map[string]CheckFunc),
	}
}

// AddCheck registers a named health check function.
// Call this for each external dependency (DB, downstream service, cache).
// Example:
//
//	h.AddCheck("database", func() health.CheckResult {
//	    if err := db.Ping(); err != nil {
//	        return health.CheckResult{Status: health.StatusUnhealthy, Message: err.Error()}
//	    }
//	    return health.CheckResult{Status: health.StatusHealthy}
//	})
func (h *Handler) AddCheck(name string, fn CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = fn
}

// runChecks executes all registered checks and aggregates the overall status.
func (h *Handler) runChecks() (Status, map[string]CheckResult) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	results := make(map[string]CheckResult, len(h.checks))
	overall := StatusHealthy

	for name, fn := range h.checks {
		result := fn()
		results[name] = result
		switch result.Status {
		case StatusUnhealthy:
			overall = StatusUnhealthy
		case StatusDegraded:
			if overall != StatusUnhealthy {
				overall = StatusDegraded
			}
		}
	}
	return overall, results
}

// buildResponse constructs the health response object.
func (h *Handler) buildResponse(overallStatus Status, checks map[string]CheckResult) Response {
	return Response{
		Status:        overallStatus,
		Service:       h.serviceName,
		Version:       h.version,
		UptimeSeconds: time.Since(h.startedAt).Seconds(),
		StartedAt:     h.startedAt.Format(time.RFC3339),
		Checks:        checks,
	}
}

// Live handles GET /health/live — Kubernetes liveness probe.
// Returns 200 always (if the process is running, it's alive).
// Does NOT run dependency checks — slow checks here would trigger
// unnecessary pod restarts.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)
	resp := h.buildResponse(StatusHealthy, map[string]CheckResult{
		"process": {Status: StatusHealthy, Message: "running"},
	})
	response.Success(w, http.StatusOK, traceID, resp)
}

// Ready handles GET /health/ready — Kubernetes readiness probe.
// Runs all registered dependency checks. Returns 503 if any check fails,
// causing Kubernetes to remove this pod from load balancer rotation.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)
	overallStatus, checks := h.runChecks()

	httpStatus := http.StatusOK
	if overallStatus == StatusUnhealthy {
		httpStatus = http.StatusServiceUnavailable
	}

	resp := h.buildResponse(overallStatus, checks)
	response.Success(w, httpStatus, traceID, resp)
}

// Health handles GET /health — combined endpoint for backward compatibility
// and for human operators. Returns full check results.
// Used as a quick manual check: curl http://localhost:8080/health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)
	overallStatus, checks := h.runChecks()

	httpStatus := http.StatusOK
	if overallStatus == StatusUnhealthy {
		httpStatus = http.StatusServiceUnavailable
	}

	resp := h.buildResponse(overallStatus, checks)
	response.Success(w, httpStatus, traceID, resp)
}

// OKCheck returns a CheckResult that always reports healthy.
// Use this as a placeholder for in-memory dependencies that cannot fail.
func OKCheck(message string) CheckFunc {
	return func() CheckResult {
		return CheckResult{Status: StatusHealthy, Message: message}
	}
}
