// services/gateway/handler/gateway.go
//
// WHY THIS EXISTS:
//   The API Gateway is the single entry point for all public traffic.
//   In Phase 3, we add:
//     - Circuit Breakers: Protect the gateway from hanging when backends are down.
//     - OpenTelemetry: Auto trace proxy requests.

package handler

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/rohithsandiri/Summer_Project/internal/shared/breaker"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
)

// RouteConfig maps a URL prefix to a backend service URL.
type RouteConfig struct {
	Prefix      string
	BackendURL  string
	StripPrefix string
}

// GatewayHandler implements the reverse proxy routing logic.
type GatewayHandler struct {
	routes    []routeEntry
	log       *logger.Logger
	startedAt time.Time
}

// routeEntry is a compiled route with a pre-built ReverseProxy.
type routeEntry struct {
	prefix      string
	stripPrefix string
	proxy       *httputil.ReverseProxy
	backendURL  string
	breaker     *breaker.CircuitBreaker
}

// New creates a GatewayHandler with the given route configurations.
func New(routes []RouteConfig, log *logger.Logger) (*GatewayHandler, error) {
	entries := make([]routeEntry, 0, len(routes))

	for _, r := range routes {
		target, err := url.Parse(r.BackendURL)
		if err != nil {
			return nil, fmt.Errorf("invalid backend URL for prefix %q: %w", r.Prefix, err)
		}

		proxy := httputil.NewSingleHostReverseProxy(target)

		// Create a Circuit Breaker for this route
		cb := breaker.New(breaker.Config{
			Name:             fmt.Sprintf("gateway-to-%s", target.Host),
			FailureThreshold: 5,
			RecoveryInterval: 15 * time.Second,
		})

		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = target.Host

			if r.StripPrefix != "" {
				req.URL.Path = stripPrefixPath(req.URL.Path, r.StripPrefix)
				req.URL.RawPath = stripPrefixPath(req.URL.RawPath, r.StripPrefix)
			}

			// Trace ID propagation
			traceID := req.Header.Get(middleware.TraceIDHeader)
			if traceID == "" {
				if v := req.Context().Value(middleware.TraceIDKey); v != nil {
					traceID = v.(string)
				}
			}
			if traceID != "" {
				req.Header.Set(middleware.TraceIDHeader, traceID)
			}

			// OpenTelemetry propagation (inject W3C traceparent context)
			otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))

			req.Header.Set("X-Forwarded-Host", req.Host)
			req.Header.Add("X-Forwarded-For", req.RemoteAddr)
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			traceID := middleware.TraceIDFromContext(req)
			log.Error("backend proxy error",
				"trace_id", traceID,
				"backend", r.BackendURL,
				"prefix", r.Prefix,
				"error", err.Error(),
			)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set(middleware.TraceIDHeader, traceID)
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"error":{"code":"BACKEND_UNAVAILABLE","message":"upstream service unavailable"},"trace_id":%q}`, traceID)
		}

		entries = append(entries, routeEntry{
			prefix:      r.Prefix,
			stripPrefix: r.StripPrefix,
			proxy:       proxy,
			backendURL:  r.BackendURL,
			breaker:     cb,
		})
	}

	return &GatewayHandler{
		routes:    entries,
		log:       log,
		startedAt: time.Now().UTC(),
	}, nil
}

// ServeHTTP implements http.Handler.
func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	for _, entry := range g.routes {
		if len(r.URL.Path) >= len(entry.prefix) &&
			r.URL.Path[:len(entry.prefix)] == entry.prefix {

			// Wrap proxy execution in the circuit breaker
			err := entry.breaker.Execute(func() error {
				// We create a custom response wrapper to capture gateway-level status errors
				// that should trip the breaker (502 Bad Gateway / 503 Unavailable).
				rw := &proxyResponseWriter{
					ResponseWriter: w,
					statusCode:     http.StatusOK,
				}

				entry.proxy.ServeHTTP(rw, r)

				// Trip the circuit breaker for upstream outages
				if rw.statusCode == http.StatusBadGateway || rw.statusCode == http.StatusServiceUnavailable {
					return fmt.Errorf("backend returned upstream failure status: %d", rw.statusCode)
				}
				return nil
			})

			if err != nil && err == breaker.ErrCircuitOpen {
				g.log.Warn("circuit_breaker_open",
					"trace_id", traceID,
					"backend", entry.backendURL,
					"path", r.URL.Path,
				)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(middleware.TraceIDHeader, traceID)
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"error":{"code":"CIRCUIT_BREAKER_OPEN","message":"service circuit breaker is currently open, please try again later"},"trace_id":%q}`, traceID)
				return
			}
			return
		}
	}

	// No route matched
	g.log.Warn("no route matched",
		"trace_id", traceID,
		"method", r.Method,
		"path", r.URL.Path,
	)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(middleware.TraceIDHeader, traceID)
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `{"error":{"code":"ROUTE_NOT_FOUND","message":"no gateway route matches the requested path"},"trace_id":%q}`, traceID)
}

func stripPrefixPath(path, prefix string) string {
	if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
		remainder := path[len(prefix):]
		if remainder == "" {
			return "/"
		}
		return remainder
	}
	return path
}

// proxyResponseWriter wraps ResponseWriter to inspect HTTP status codes returned by the proxy
type proxyResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (prw *proxyResponseWriter) WriteHeader(code int) {
	prw.statusCode = code
	prw.ResponseWriter.WriteHeader(code)
}
