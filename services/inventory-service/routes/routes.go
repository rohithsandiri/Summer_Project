// services/inventory-service/routes/routes.go
//
// Updated to pass serviceName to TraceID middleware to start OTel server spans.

package routes

import (
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/handler"
)

// Register wires all inventory routes onto the provided mux.
func Register(
	mux *http.ServeMux,
	h *handler.InventoryHandler,
	m *metrics.Metrics,
	log *logger.Logger,
	serviceName string,
) {
	chain := func(next http.HandlerFunc) http.Handler {
		return middleware.Chain(next,
			middleware.TraceID(serviceName), // Updated
			middleware.Logging(log),
			middleware.MetricsMiddleware(m, serviceName),
			middleware.Recovery(log),
		)
	}

	mux.Handle("GET /inventory/{item}", chain(h.GetItem))
	mux.Handle("POST /inventory/reserve", chain(h.Reserve))
	mux.Handle("POST /inventory/release", chain(h.Release))

	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /health/live", h.Live)
	mux.HandleFunc("GET /health/ready", h.Ready)
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))

	// Profiling endpoints
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}
