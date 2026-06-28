// cmd/order-service/main.go
//
// Composition root for the Order Service.
// In Phase 3, we add:
//   - OpenTelemetry OTLP Tracer
//   - Redis Integration (idempotency + query cache)
//   - Circuit Breakers for downstream calls (Inventory and Payment)

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rohithsandiri/Summer_Project/internal/shared/breaker"
	"github.com/rohithsandiri/Summer_Project/internal/shared/config"
	"github.com/rohithsandiri/Summer_Project/internal/shared/health"
	"github.com/rohithsandiri/Summer_Project/internal/shared/httpclient"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/shared/otel"
	redisclient "github.com/rohithsandiri/Summer_Project/internal/shared/redis"
	"github.com/rohithsandiri/Summer_Project/services/order-service/handler"
	"github.com/rohithsandiri/Summer_Project/services/order-service/repository"
	"github.com/rohithsandiri/Summer_Project/services/order-service/routes"
	"github.com/rohithsandiri/Summer_Project/services/order-service/service"
)

func main() {
	// ── 1. Configuration (fail-fast validation) ───────────────────────────────
	cfg, err := config.Load("order-service", config.LoadOptions{
		RequireInventoryURL: true,
		RequirePaymentURL:   true,
		RequireRedis:        true,
	})
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	// ── 2. Logger ─────────────────────────────────────────────────────────────
	log_ := logger.New(cfg.ServiceName, cfg.LogLevel)
	log_.Info("starting service",
		"port", cfg.Port,
		"version", cfg.Version,
		"inventory_url", cfg.InventoryServiceURL,
		"payment_url", cfg.PaymentServiceURL,
		"redis_url", cfg.RedisURL,
		"otel_endpoint", cfg.OTelExporterEndpoint,
	)

	// ── 3. OpenTelemetry Tracer Initialization ────────────────────────────────
	otelCtx, otelCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer otelCancel()
	otelShutdown, err := otel.InitTracer(otelCtx, cfg.ServiceName, cfg.Version, cfg.OTelExporterEndpoint)
	if err != nil {
		log_.Warn("failed to initialize OpenTelemetry tracing", "error", err.Error())
	}

	// ── 4. Redis Client Initialization (fail-fast check) ──────────────────────
	rdb, err := redisclient.NewClient(cfg.RedisURL, cfg.RedisPassword, 0, cfg.ServiceName)
	if err != nil {
		log.Fatalf("FATAL: failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	// ── 5. Infrastructure Metrics ─────────────────────────────────────────────
	m := metrics.New(cfg.ServiceName)

	// ── 6. Business Metrics ───────────────────────────────────────────────────
	bm := metrics.NewOrderMetrics(m.Registry())

	// ── 7. Repository ─────────────────────────────────────────────────────────
	repo := repository.NewInMemoryOrderRepository()

	// ── 8. Circuit Breakers for Downstream Calls ──────────────────────────────
	// Create circuit breakers for downstream inventory and payment services
	inventoryBreaker := breaker.New(breaker.Config{
		Name:             "order-to-inventory",
		FailureThreshold: 5,
		RecoveryInterval: 20 * time.Second,
	})

	paymentBreaker := breaker.New(breaker.Config{
		Name:             "order-to-payment",
		FailureThreshold: 5,
		RecoveryInterval: 20 * time.Second,
	})

	// ── 9. Downstream Service Clients with Retry & Breaker ────────────────────
	inventoryClient := httpclient.New(cfg.InventoryServiceURL, "inventory-service").SetBreaker(inventoryBreaker)
	paymentClient := httpclient.New(cfg.PaymentServiceURL, "payment-service").SetBreaker(paymentBreaker)

	// ── 10. Service Layer ─────────────────────────────────────────────────────
	svc := service.New(repo, inventoryClient, paymentClient, log_, bm, rdb)

	// ── 11. Health Checks ─────────────────────────────────────────────────────
	healthHandler := health.NewHandler(cfg.ServiceName, cfg.Version)
	healthHandler.AddCheck("repository", health.OKCheck("in-memory"))
	healthHandler.AddCheck("redis", func() health.CheckResult {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx); err != nil {
			return health.CheckResult{Status: health.StatusUnhealthy, Message: err.Error()}
		}
		return health.CheckResult{Status: health.StatusHealthy, Message: "connected"}
	})
	healthHandler.AddCheck("inventory_service", health.OKCheck(cfg.InventoryServiceURL))
	healthHandler.AddCheck("payment_service", health.OKCheck(cfg.PaymentServiceURL))

	// ── 12. Handler + Routes ──────────────────────────────────────────────────
	h := handler.New(svc, healthHandler)
	mux := http.NewServeMux()
	routes.Register(mux, h, m, log_, cfg.ServiceName)

	// ── 13. HTTP Server + Graceful Shutdown ───────────────────────────────────
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log_.Info("http server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log_.Info("shutdown signal received", "signal", sig.String())

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
		log_.Info("server shut down cleanly")
	}
}
