// cmd/gateway/main.go
//
// Composition root for the API Gateway.
// In Phase 3, we add rate limiting, OpenTelemetry distributed tracing,
// and security header filtering.

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rohithsandiri/Summer_Project/internal/shared/config"
	"github.com/rohithsandiri/Summer_Project/internal/shared/health"
	"github.com/rohithsandiri/Summer_Project/internal/shared/limiter"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/otel"
	"github.com/rohithsandiri/Summer_Project/services/gateway/handler"
)

func main() {
	// ── 1. Configuration ─────────────────────────────────────────────────────
	cfg, err := config.Load("api-gateway", config.LoadOptions{
		RequireOrderURL:     true,
		RequireInventoryURL: true,
		RequirePaymentURL:   true,
	})
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	// ── 2. Logger ─────────────────────────────────────────────────────────────
	log_ := logger.New(cfg.ServiceName, cfg.LogLevel)
	log_.Info("starting api gateway",
		"port", cfg.GatewayPort,
		"order_url", cfg.OrderServiceURL,
		"inventory_url", cfg.InventoryServiceURL,
		"payment_url", cfg.PaymentServiceURL,
		"otel_endpoint", cfg.OTelExporterEndpoint,
	)

	// ── 3. OpenTelemetry Tracer Initialization ────────────────────────────────
	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	otelShutdown, err := otel.InitTracer(otelCtx, cfg.ServiceName, cfg.Version, cfg.OTelExporterEndpoint)
	if err != nil {
		log_.Warn("failed to initialize OpenTelemetry tracing", "error", err.Error())
	}

	// ── 4. Metrics ────────────────────────────────────────────────────────────
	m := metrics.New(cfg.ServiceName)

	// ── 5. Rate Limiter ───────────────────────────────────────────────────────
	rl := limiter.NewRateLimiter(cfg.ServiceName, cfg.RateLimitRPS, cfg.RateLimitBurst)

	// ── 6. Health ─────────────────────────────────────────────────────────────
	healthHandler := health.NewHandler(cfg.ServiceName, cfg.Version)
	healthHandler.AddCheck("order_service_configured", health.OKCheck(cfg.OrderServiceURL))
	healthHandler.AddCheck("inventory_service_configured", health.OKCheck(cfg.InventoryServiceURL))
	healthHandler.AddCheck("payment_service_configured", health.OKCheck(cfg.PaymentServiceURL))

	// ── 7. Gateway Reverse Proxy ──────────────────────────────────────────────
	routes := []handler.RouteConfig{
		{
			Prefix:      "/api/orders",
			BackendURL:  cfg.OrderServiceURL,
			StripPrefix: "/api",
		},
		{
			Prefix:      "/api/inventory",
			BackendURL:  cfg.InventoryServiceURL,
			StripPrefix: "/api",
		},
		{
			Prefix:      "/api/payments",
			BackendURL:  cfg.PaymentServiceURL,
			StripPrefix: "/api",
		},
	}

	gatewayHandler, err := handler.New(routes, log_)
	if err != nil {
		log.Fatalf("FATAL: failed to build gateway routes: %v", err)
	}

	// ── 8. Mux + Middleware Chain ─────────────────────────────────────────────
	mux := http.NewServeMux()

	// Apply Rate Limiting first, then proxy gateway logic
	rateLimitedGateway := rl.LimitHandler(log_)(gatewayHandler)

	// Outermost middleware chain for API traffic
	apiChain := func(h http.Handler) http.Handler {
		return middleware.Chain(h,
			middleware.TraceID(cfg.ServiceName),
			middleware.Logging(log_),
			middleware.MetricsMiddleware(m, cfg.ServiceName),
			middleware.SecurityHeaders,
			middleware.RequestSizeLimiter(1<<20), // 1MB payload size limit
			middleware.Recovery(log_),
		)
	}

	mux.Handle("/api/", apiChain(rateLimitedGateway))

	// Infra routes (not rate limited or security-processed)
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("GET /health/live", healthHandler.Live)
	mux.HandleFunc("GET /health/ready", healthHandler.Ready)
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))

	// Profiling endpoints
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	// ── 9. HTTP Server + Graceful Shutdown ────────────────────────────────────
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.GatewayPort),
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  90 * time.Second,
	}

	go func() {
		log_.Info("gateway listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log_.Info("shutdown signal received", "signal", sig.String())

	// Start graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if otelShutdown != nil {
		log_.Info("flushing and shutting down OpenTelemetry tracer...")
		if err := otelShutdown(ctx); err != nil {
			log_.Error("failed to shutdown tracer", "error", err)
		}
	}

	if err := server.Shutdown(ctx); err != nil {
		log_.Error("graceful shutdown failed", "error", err)
	} else {
		log_.Info("gateway shut down cleanly")
	}
}
