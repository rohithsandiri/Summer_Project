// services/payment-service/service/service.go
//
// Updated to support:
//   1. OpenTelemetry custom business spans.
//   2. Redis-based payment caching (Part 9).

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	redisclient "github.com/rohithsandiri/Summer_Project/internal/shared/redis"
	"github.com/rohithsandiri/Summer_Project/services/payment-service/models"
	"github.com/rohithsandiri/Summer_Project/services/payment-service/repository"
)

type PaymentService interface {
	Process(ctx context.Context, req *models.ProcessRequest) (*models.ProcessResponse, error)
	Refund(ctx context.Context, req *models.RefundRequest) (*models.RefundResponse, error)
	GetPayment(ctx context.Context, paymentID string) (*models.Payment, error)
}

type paymentService struct {
	repo    repository.PaymentRepository
	log     *logger.Logger
	metrics *metrics.PaymentMetrics
	rdb     *redisclient.Client
}

func New(repo repository.PaymentRepository, log *logger.Logger, m *metrics.PaymentMetrics, rdb *redisclient.Client) PaymentService {
	return &paymentService{repo: repo, log: log, metrics: m, rdb: rdb}
}

func (s *paymentService) Process(ctx context.Context, req *models.ProcessRequest) (*models.ProcessResponse, error) {
	tracer := otel.Tracer("payment-service")
	ctx, span := tracer.Start(ctx, "ProcessPayment", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("order_id", req.OrderID),
		attribute.Int64("payment_amount", req.Amount),
		attribute.String("payment_currency", req.Currency),
	)

	traceID := middleware.TraceIDFromCtx(ctx)

	if req.OrderID == "" {
		return nil, apperrors.ErrPaymentValidation.WithMessage("order_id is required")
	}
	if req.Amount <= 0 {
		return nil, apperrors.ErrPaymentValidation.WithMessage(
			fmt.Sprintf("amount must be positive, got %d", req.Amount),
		)
	}
	if req.Currency == "" {
		return nil, apperrors.ErrPaymentValidation.WithMessage("currency is required")
	}

	now := time.Now().UTC()
	paymentID := fmt.Sprintf("pay_%d", now.UnixNano())

	gatewayStart := time.Now()
	status, failureReason := s.simulateGateway()
	gatewayDuration := time.Since(gatewayStart)

	if s.metrics != nil {
		s.metrics.PaymentProcessingDuration.Observe(gatewayDuration.Seconds())
	}

	payment := &models.Payment{
		PaymentID:     paymentID,
		OrderID:       req.OrderID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Status:        status,
		FailureReason: failureReason,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.repo.Create(payment); err != nil {
		span.RecordError(err)
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	span.SetAttributes(
		attribute.String("payment_id", paymentID),
		attribute.String("payment_status", string(status)),
	)

	if status == models.PaymentStatusSucceeded {
		if s.metrics != nil {
			s.metrics.PaymentsSuccessTotal.Inc()
		}
		s.log.Info("payment succeeded",
			"trace_id", traceID,
			"payment_id", paymentID,
			"order_id", req.OrderID,
			"amount", req.Amount,
			"currency", req.Currency,
		)
	} else {
		if s.metrics != nil {
			s.metrics.PaymentsFailedTotal.WithLabelValues(failureReason).Inc()
		}
		s.log.Warn("payment declined",
			"trace_id", traceID,
			"payment_id", paymentID,
			"order_id", req.OrderID,
			"reason", failureReason,
		)
	}

	// Write back to cache (Part 9)
	cacheKey := fmt.Sprintf("cache:payment:%s", paymentID)
	payBytes, err := json.Marshal(payment)
	if err == nil {
		_ = s.rdb.Set(ctx, cacheKey, string(payBytes), 5*time.Minute)
	}

	return &models.ProcessResponse{
		PaymentID:     paymentID,
		OrderID:       req.OrderID,
		Status:        status,
		Amount:        req.Amount,
		Currency:      req.Currency,
		FailureReason: failureReason,
		ProcessedAt:   now,
	}, nil
}

func (s *paymentService) Refund(ctx context.Context, req *models.RefundRequest) (*models.RefundResponse, error) {
	tracer := otel.Tracer("payment-service")
	ctx, span := tracer.Start(ctx, "RefundPayment", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("payment_id", req.PaymentID),
		attribute.String("order_id", req.OrderID),
	)

	traceID := middleware.TraceIDFromCtx(ctx)

	if req.PaymentID == "" {
		return nil, apperrors.ErrPaymentValidation.WithMessage("payment_id is required")
	}
	if req.OrderID == "" {
		return nil, apperrors.ErrPaymentValidation.WithMessage("order_id is required")
	}

	payment, err := s.repo.GetByID(req.PaymentID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if payment.OrderID != req.OrderID {
		return nil, apperrors.ErrPaymentValidation.WithMessage(
			"payment does not belong to this order",
		)
	}

	if payment.Status == models.PaymentStatusRefunded {
		return nil, apperrors.ErrPaymentAlreadyRefunded
	}

	if payment.Status != models.PaymentStatusSucceeded {
		return nil, apperrors.ErrPaymentNotRefundable.WithMessage(
			fmt.Sprintf("cannot refund payment in %s state", payment.Status),
		)
	}

	if err := s.repo.UpdateStatus(req.PaymentID, models.PaymentStatusRefunded, ""); err != nil {
		span.RecordError(err)
		return nil, err
	}

	s.log.Info("payment refunded",
		"trace_id", traceID,
		"payment_id", req.PaymentID,
		"order_id", req.OrderID,
	)

	// Invalidate Cache since state changed
	cacheKey := fmt.Sprintf("cache:payment:%s", req.PaymentID)
	_ = s.rdb.Delete(ctx, cacheKey)

	return &models.RefundResponse{
		PaymentID:  req.PaymentID,
		OrderID:    req.OrderID,
		Status:     models.PaymentStatusRefunded,
		RefundedAt: time.Now().UTC(),
	}, nil
}

func (s *paymentService) GetPayment(ctx context.Context, paymentID string) (*models.Payment, error) {
	tracer := otel.Tracer("payment-service")
	ctx, span := tracer.Start(ctx, "GetPayment", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(attribute.String("payment_id", paymentID))

	if paymentID == "" {
		return nil, apperrors.ErrPaymentValidation.WithMessage("payment_id is required")
	}

	// 1. Query Redis Caching (Part 9)
	cacheKey := fmt.Sprintf("cache:payment:%s", paymentID)
	cachedVal, err := s.rdb.Get(ctx, "payment_cache", cacheKey)
	if err == nil && cachedVal != "" {
		var payment models.Payment
		if err := json.Unmarshal([]byte(cachedVal), &payment); err == nil {
			s.log.Info("returning cached payment details", "payment_id", paymentID)
			span.SetAttributes(attribute.Bool("cache_hit", true))
			return &payment, nil
		}
	}

	payment, err := s.repo.GetByID(paymentID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Write back to cache
	payBytes, err := json.Marshal(payment)
	if err == nil {
		_ = s.rdb.Set(ctx, cacheKey, string(payBytes), 5*time.Minute)
	}

	return payment, nil
}

func (s *paymentService) simulateGateway() (models.PaymentStatus, string) {
	failureRate := getEnvFloat("PAYMENT_FAILURE_RATE", 0.1)
	if rand.Float64() < failureRate {
		reasons := []string{"insufficient_funds", "card_declined", "gateway_timeout", "fraud_detected"}
		return models.PaymentStatusFailed, reasons[rand.Intn(len(reasons))]
	}
	return models.PaymentStatusSucceeded, ""
}

func getEnvFloat(key string, defaultVal float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return f
}
