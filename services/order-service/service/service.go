// services/order-service/service/service.go
//
// Updated to support:
//   1. OpenTelemetry Business Spans: trace the saga execution and sub-calls.
//   2. Redis Idempotency: prevent double-submit using a lock/cache key in Redis.
//   3. Circuit Breaker integration (via httpclient wrapper).

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/internal/shared/httpclient"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	redisclient "github.com/rohithsandiri/Summer_Project/internal/shared/redis"
	"github.com/rohithsandiri/Summer_Project/services/order-service/models"
	"github.com/rohithsandiri/Summer_Project/services/order-service/repository"
)

type inventoryReserveRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	OrderID  string `json:"order_id"`
}

type inventoryReleaseRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	OrderID  string `json:"order_id"`
}

type paymentProcessRequest struct {
	OrderID  string `json:"order_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type paymentProcessData struct {
	PaymentID     string `json:"payment_id"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason"`
}

type paymentProcessEnvelope struct {
	Data paymentProcessData `json:"data"`
}

type OrderService interface {
	CreateOrder(ctx context.Context, traceID string, req *models.CreateOrderRequest) (*models.CreateOrderResponse, error)
	GetOrder(ctx context.Context, orderID string) (*models.Order, error)
}

type orderService struct {
	repo            repository.OrderRepository
	inventoryClient *httpclient.Client
	paymentClient   *httpclient.Client
	log             *logger.Logger
	metrics         *metrics.OrderMetrics
	rdb             *redisclient.Client
}

func New(
	repo repository.OrderRepository,
	inventoryClient *httpclient.Client,
	paymentClient *httpclient.Client,
	log *logger.Logger,
	m *metrics.OrderMetrics,
	rdb *redisclient.Client,
) OrderService {
	return &orderService{
		repo:            repo,
		inventoryClient: inventoryClient,
		paymentClient:   paymentClient,
		log:             log,
		metrics:         m,
		rdb:             rdb,
	}
}

func (s *orderService) CreateOrder(ctx context.Context, traceID string, req *models.CreateOrderRequest) (*models.CreateOrderResponse, error) {
	sagaStart := time.Now()
	// Start OpenTelemetry business span for the saga
	tracer := otel.Tracer("order-service")
	ctx, span := tracer.Start(ctx, "CreateOrderSaga", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	// ── 1. Validate ──
	if err := validateCreateRequest(req); err != nil {
		span.RecordError(err)
		span.SetStatus(1, "validation failed")
		return nil, err
	}

	// ── 2. Idempotency Check (Redis integration) ──
	var idempotencyKey string
	if req.IdempotencyKey != "" {
		idempotencyKey = fmt.Sprintf("idempotency:order:%s", req.IdempotencyKey)
		span.SetAttributes(attribute.String("idempotency_key", req.IdempotencyKey))

		// Try to acquire idempotency lock / cache entry
		isNew, err := s.rdb.SetNX(ctx, idempotencyKey, "PROCESSING", 10*time.Minute)
		if err != nil {
			s.log.Warn("idempotency check failed in redis", "error", err.Error())
		} else if !isNew {
			// Key exists: check if processing or already completed
			cachedVal, err := s.rdb.Get(ctx, "idempotency_cache", idempotencyKey)
			if err == nil && cachedVal != "" {
				if cachedVal == "PROCESSING" {
					return nil, apperrors.ErrDuplicateOrder.WithMessage("request is currently being processed, please wait")
				}
				// Return cached response
				var cachedResp models.CreateOrderResponse
				if err := json.Unmarshal([]byte(cachedVal), &cachedResp); err == nil {
					s.log.Info("returning cached response (idempotency)", "key", req.IdempotencyKey)
					span.SetAttributes(attribute.Bool("idempotency_hit", true))
					return &cachedResp, nil
				}
			}
			return nil, apperrors.ErrDuplicateOrder.WithMessage("duplicate order submission detected")
		}
	}

	// Helper to clean up or cache outcome on exit
	defer func() {
		if idempotencyKey != "" {
			// If we panicked or failed, delete key so user can try again
			// In a real database transaction, we'd roll back
		}
	}()

	now := time.Now().UTC()
	orderID := fmt.Sprintf("ord_%d", now.UnixNano())
	total := calculateTotal(req.Items)
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	span.SetAttributes(
		attribute.String("order_id", orderID),
		attribute.Int64("order_total", total),
		attribute.String("order_currency", currency),
	)

	// ── 3. Persist order as PENDING ──
	order := &models.Order{
		OrderID:     orderID,
		CustomerID:  req.CustomerID,
		Items:       req.Items,
		TotalAmount: total,
		Currency:    currency,
		Status:      models.OrderStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.Create(order); err != nil {
		span.RecordError(err)
		if idempotencyKey != "" {
			_ = s.rdb.Delete(ctx, idempotencyKey)
		}
		return nil, err
	}

	log := s.log.With("order_id", orderID, "trace_id", traceID)
	log.Info("order_pending", "items", len(req.Items), "total", total, "currency", currency)

	// ── 4. Reserve inventory ──
	reservedItems := make([]models.OrderItem, 0, len(req.Items))

	for _, item := range req.Items {
		log.Info("reserving_inventory", "item_id", item.ItemID, "quantity", item.Quantity)

		// Sub-span for inventory reservation call
		invCtx, invSpan := tracer.Start(ctx, "InventoryReserveCall", oteltrace.WithSpanKind(oteltrace.SpanKindClient))
		invSpan.SetAttributes(
			attribute.String("item_id", item.ItemID),
			attribute.Int("quantity", item.Quantity),
		)

		statusCode, err := s.inventoryClient.PostJSON(invCtx, "/inventory/reserve", traceID,
			inventoryReserveRequest{
				ItemID:   item.ItemID,
				Quantity: item.Quantity,
				OrderID:  orderID,
			}, nil)

		invSpan.SetAttributes(attribute.Int("http.status_code", statusCode))
		if err != nil {
			invSpan.RecordError(err)
			invSpan.End()

			reason := fmt.Sprintf("inventory service call failed: %v", err)
			s.releaseReservedItems(ctx, traceID, orderID, reservedItems)

			orderErr := s.failOrder(order, reason, "inventory_unavailable")
			if idempotencyKey != "" {
				_ = s.rdb.Delete(ctx, idempotencyKey)
			}
			return nil, orderErr
		}
		invSpan.End()

		if statusCode != 200 {
			reason := fmt.Sprintf("inventory reservation failed for item %s (HTTP %d)", item.ItemID, statusCode)
			s.releaseReservedItems(ctx, traceID, orderID, reservedItems)

			orderErr := s.failOrder(order, reason, "inventory_unavailable")
			if idempotencyKey != "" {
				_ = s.rdb.Delete(ctx, idempotencyKey)
			}
			return nil, orderErr
		}

		reservedItems = append(reservedItems, item)
	}

	// ── 5. Process payment ──
	log.Info("processing_payment", "amount", total, "currency", currency)

	var payResp paymentProcessEnvelope
	payCtx, paySpan := tracer.Start(ctx, "PaymentProcessCall", oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	paySpan.SetAttributes(attribute.Int64("payment_amount", total))

	statusCode, err := s.paymentClient.PostJSON(payCtx, "/payment/process", traceID,
		paymentProcessRequest{
			OrderID:  orderID,
			Amount:   total,
			Currency: currency,
		}, &payResp)

	paySpan.SetAttributes(
		attribute.Int("http.status_code", statusCode),
		attribute.String("payment_status", payResp.Data.Status),
	)

	if err != nil {
		paySpan.RecordError(err)
		paySpan.End()

		reason := fmt.Sprintf("payment service call failed: %v", err)
		s.releaseReservedItems(ctx, traceID, orderID, reservedItems)

		orderErr := s.failOrder(order, reason, "payment_service_unavailable")
		if idempotencyKey != "" {
			_ = s.rdb.Delete(ctx, idempotencyKey)
		}
		return nil, orderErr
	}
	paySpan.End()

	if statusCode != 200 {
		reason := fmt.Sprintf("payment service returned HTTP %d", statusCode)
		s.releaseReservedItems(ctx, traceID, orderID, reservedItems)

		orderErr := s.failOrder(order, reason, "payment_service_error")
		if idempotencyKey != "" {
			_ = s.rdb.Delete(ctx, idempotencyKey)
		}
		return nil, orderErr
	}

	if payResp.Data.Status != "SUCCEEDED" {
		reason := fmt.Sprintf("payment declined: %s", payResp.Data.FailureReason)
		s.releaseReservedItems(ctx, traceID, orderID, reservedItems)

		orderErr := s.failOrder(order, reason, "payment_declined")
		if idempotencyKey != "" {
			_ = s.rdb.Delete(ctx, idempotencyKey)
		}
		return nil, orderErr
	}

	// ── 6. Confirm order ──
	order.Status = models.OrderStatusConfirmed
	order.PaymentID = payResp.Data.PaymentID
	order.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(order); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Record metrics
	sagaDuration := time.Since(sagaStart)
	if s.metrics != nil {
		s.metrics.OrdersCreatedTotal.Inc()
		s.metrics.OrderProcessingDuration.Observe(sagaDuration.Seconds())
	}

	log.Info("order_confirmed",
		"payment_id", order.PaymentID,
		"saga_duration_ms", sagaDuration.Milliseconds(),
	)

	resp := &models.CreateOrderResponse{
		Order:   order,
		Message: "order confirmed successfully",
	}

	// ── 7. Cache outcome in Redis (idempotency) ──
	if idempotencyKey != "" {
		respBytes, err := json.Marshal(resp)
		if err == nil {
			_ = s.rdb.Set(ctx, idempotencyKey, string(respBytes), 24*time.Hour)
		}
	}

	span.SetStatus(2, "order confirmed")
	return resp, nil
}

func (s *orderService) GetOrder(ctx context.Context, orderID string) (*models.Order, error) {
	// OpenTelemetry tracing span
	tracer := otel.Tracer("order-service")
	ctx, span := tracer.Start(ctx, "GetOrder", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(attribute.String("order_id", orderID))

	if orderID == "" {
		return nil, apperrors.ErrOrderValidation.WithMessage("order_id is required")
	}

	// Caching implementation for reads (Part 9)
	cacheKey := fmt.Sprintf("cache:order:%s", orderID)
	cachedVal, err := s.rdb.Get(ctx, "order_cache", cacheKey)
	if err == nil && cachedVal != "" {
		var order models.Order
		if err := json.Unmarshal([]byte(cachedVal), &order); err == nil {
			s.log.Info("returning cached order details", "order_id", orderID)
			return &order, nil
		}
	}

	order, err := s.repo.GetByID(orderID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Store in cache for 5 minutes
	orderBytes, err := json.Marshal(order)
	if err == nil {
		_ = s.rdb.Set(ctx, cacheKey, string(orderBytes), 5*time.Minute)
	}

	return order, nil
}

func (s *orderService) releaseReservedItems(ctx context.Context, traceID, orderID string, items []models.OrderItem) {
	tracer := otel.Tracer("order-service")
	for _, item := range items {
		compCtx, compSpan := tracer.Start(ctx, "CompensationInventoryRelease", oteltrace.WithSpanKind(oteltrace.SpanKindClient))
		compSpan.SetAttributes(
			attribute.String("item_id", item.ItemID),
			attribute.Int("quantity", item.Quantity),
		)

		_, err := s.inventoryClient.PostJSON(compCtx, "/inventory/release", traceID,
			inventoryReleaseRequest{
				ItemID:   item.ItemID,
				Quantity: item.Quantity,
				OrderID:  orderID,
			}, nil)
		if err != nil {
			compSpan.RecordError(err)
			s.log.Error("compensation_release_failed",
				"trace_id", traceID,
				"item_id", item.ItemID,
				"order_id", orderID,
				"error", err.Error(),
			)
		} else {
			s.log.Info("compensation_released",
				"trace_id", traceID,
				"item_id", item.ItemID,
				"order_id", orderID,
			)
		}
		compSpan.End()
	}
}

func (s *orderService) failOrder(order *models.Order, reason, metricLabel string) error {
	order.Status = models.OrderStatusFailed
	order.FailureReason = reason
	order.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(order); err != nil {
		s.log.Error("persist_order_failure_failed",
			"order_id", order.OrderID,
			"error", err.Error(),
		)
	}

	if s.metrics != nil {
		s.metrics.OrdersFailedTotal.WithLabelValues(metricLabel).Inc()
	}

	return apperrors.ErrOrderFailed.WithMessage(
		fmt.Sprintf("order %s failed: %s", order.OrderID, reason),
	)
}

func validateCreateRequest(req *models.CreateOrderRequest) error {
	if req.CustomerID == "" {
		return apperrors.ErrOrderValidation.WithMessage("customer_id is required")
	}
	if len(req.Items) == 0 {
		return apperrors.ErrOrderValidation.WithMessage("items must not be empty")
	}
	for i, item := range req.Items {
		if item.ItemID == "" {
			return apperrors.ErrOrderValidation.WithMessage(
				fmt.Sprintf("items[%d].item_id is required", i),
			)
		}
		if item.Quantity <= 0 {
			return apperrors.ErrOrderValidation.WithMessage(
				fmt.Sprintf("items[%d].quantity must be positive, got %d", i, item.Quantity),
			)
		}
		if item.UnitPrice < 0 {
			return apperrors.ErrOrderValidation.WithMessage(
				fmt.Sprintf("items[%d].unit_price must not be negative", i),
			)
		}
	}
	return nil
}

func calculateTotal(items []models.OrderItem) int64 {
	var total int64
	for _, item := range items {
		total += int64(item.Quantity) * item.UnitPrice
	}
	return total
}
