// internal/shared/metrics/metrics.go
//
// WHY THIS EXISTS:
//   Prometheus scrapes /metrics from each service. This package centralises
//   ALL metric declarations — both infrastructure metrics (HTTP) and business
//   metrics (orders, payments, inventory).
//
// TWO TIERS OF METRICS:
//
//   Tier 1 — Infrastructure metrics (all services):
//     http_requests_total, http_request_duration_seconds,
//     http_requests_in_flight, http_errors_total
//
//   Tier 2 — Business metrics (service-specific):
//     Order:     orders_created_total, orders_failed_total
//     Inventory: inventory_reserved_total, inventory_available_gauge,
//                inventory_released_total
//     Payment:   payments_success_total, payments_failed_total,
//                payment_processing_duration_seconds
//
// WHY BUSINESS METRICS MATTER FOR FUTURE PHASES:
//   These become the basis for SLOs in Phase 3:
//     - SLO: 99.5% of orders must be confirmed (orders_created_total vs orders_failed_total)
//     - SLO: Payment success rate >= 95% (payments_success_total / all payments)
//   Alertmanager rules will fire when these SLOs are violated.
//   The Kubernetes Operator will read these to trigger rollbacks.
//
// DESIGN:
//   BusinessMetrics is a separate struct so services that don't have specific
//   business metrics (e.g. gateway) don't need to instantiate them.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds infrastructure Prometheus instruments shared by all services.
type Metrics struct {
	// RequestsTotal counts every completed HTTP request.
	RequestsTotal *prometheus.CounterVec

	// RequestDuration is a histogram of request latencies in seconds.
	// Buckets are tuned for web APIs; used for SLO latency targets.
	RequestDuration *prometheus.HistogramVec

	// RequestsInFlight is the current in-flight request count (gauge).
	RequestsInFlight *prometheus.GaugeVec

	// ErrorsTotal counts only error responses (status >= 400).
	ErrorsTotal *prometheus.CounterVec

	registry *prometheus.Registry
}

// OrderMetrics holds business metrics for the Order Service.
// These are the primary SLO indicators for order reliability.
type OrderMetrics struct {
	// OrdersCreatedTotal counts every successfully confirmed order.
	// SLO basis: orders_created_total / (orders_created_total + orders_failed_total) >= 0.995
	OrdersCreatedTotal prometheus.Counter

	// OrdersFailedTotal counts every order that ended in FAILED state.
	// Segmented by reason: "inventory_unavailable", "payment_declined", "internal_error"
	OrdersFailedTotal *prometheus.CounterVec

	// OrderProcessingDuration measures end-to-end order saga duration.
	// Used to detect downstream latency degradation before it breaches SLOs.
	OrderProcessingDuration prometheus.Histogram
}

// InventoryMetrics holds business metrics for the Inventory Service.
type InventoryMetrics struct {
	// InventoryReservedTotal counts successful stock reservations.
	InventoryReservedTotal *prometheus.CounterVec

	// InventoryReleasedTotal counts stock releases (compensation events).
	// High values relative to reserved indicate frequent order failures.
	InventoryReleasedTotal *prometheus.CounterVec

	// InventoryAvailable is a gauge of currently available (unreserved) units.
	// Monitored for low-stock alerting. Requires periodic sampling.
	InventoryAvailable *prometheus.GaugeVec
}

// PaymentMetrics holds business metrics for the Payment Service.
type PaymentMetrics struct {
	// PaymentsSuccessTotal counts successfully processed payments.
	PaymentsSuccessTotal prometheus.Counter

	// PaymentsFailedTotal counts failed/declined payments, by failure reason.
	PaymentsFailedTotal *prometheus.CounterVec

	// PaymentProcessingDuration measures gateway call duration.
	// P95 and P99 of this histogram map directly to payment SLO latency targets.
	PaymentProcessingDuration prometheus.Histogram
}

// New creates and registers all infrastructure metrics for a service.
func New(serviceName string) *Metrics {
	registry := prometheus.NewRegistry()

	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	factory := promauto.With(registry)

	return &Metrics{
		registry: registry,

		RequestsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total HTTP requests by service, method, path, and status code.",
			},
			[]string{"service", "method", "path", "status_code"},
		),

		RequestDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency distribution in seconds.",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"service", "method", "path"},
		),

		RequestsInFlight: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Current number of HTTP requests being processed.",
			},
			[]string{"service"},
		),

		ErrorsTotal: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_errors_total",
				Help: "Total HTTP error responses (status >= 400).",
			},
			[]string{"service", "method", "path", "status_code"},
		),
	}
}

// NewOrderMetrics creates business metrics for the Order Service.
// Pass the same registry used by the infrastructure Metrics so /metrics
// exposes both tiers in one scrape.
func NewOrderMetrics(registry *prometheus.Registry) *OrderMetrics {
	factory := promauto.With(registry)
	return &OrderMetrics{
		OrdersCreatedTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "orders_created_total",
			Help: "Total number of successfully confirmed orders. SLO numerator.",
		}),

		OrdersFailedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "orders_failed_total",
			Help: "Total failed orders segmented by failure reason.",
		}, []string{"reason"}),

		OrderProcessingDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "order_processing_duration_seconds",
			Help:    "End-to-end order saga duration (from receipt to confirm/fail).",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0},
		}),
	}
}

// NewInventoryMetrics creates business metrics for the Inventory Service.
func NewInventoryMetrics(registry *prometheus.Registry) *InventoryMetrics {
	factory := promauto.With(registry)
	return &InventoryMetrics{
		InventoryReservedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "inventory_reserved_total",
			Help: "Total successful inventory reservations by item ID.",
		}, []string{"item_id"}),

		InventoryReleasedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "inventory_released_total",
			Help: "Total inventory releases (Saga compensation events).",
		}, []string{"item_id"}),

		InventoryAvailable: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "inventory_available_units",
			Help: "Currently available (unreserved) stock units by item ID.",
		}, []string{"item_id"}),
	}
}

// NewPaymentMetrics creates business metrics for the Payment Service.
func NewPaymentMetrics(registry *prometheus.Registry) *PaymentMetrics {
	factory := promauto.With(registry)
	return &PaymentMetrics{
		PaymentsSuccessTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "payments_success_total",
			Help: "Total successfully processed payments.",
		}),

		PaymentsFailedTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "payments_failed_total",
			Help: "Total failed/declined payments by failure reason.",
		}, []string{"reason"}),

		PaymentProcessingDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "payment_processing_duration_seconds",
			Help:    "Duration of payment gateway processing calls.",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0},
		}),
	}
}

// Registry returns the prometheus.Registry for use with promhttp.HandlerFor.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}
