// services/inventory-service/service/service.go
//
// Updated to support:
//   1. OpenTelemetry custom business spans.
//   2. Redis-based inventory caching (Part 9).

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
	"github.com/rohithsandiri/Summer_Project/internal/shared/logger"
	"github.com/rohithsandiri/Summer_Project/internal/shared/metrics"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	redisclient "github.com/rohithsandiri/Summer_Project/internal/shared/redis"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/models"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/repository"
)

type InventoryService interface {
	GetItem(ctx context.Context, itemID string) (*models.InventoryItem, error)
	Reserve(ctx context.Context, req *models.ReserveRequest) (*models.ReserveResponse, error)
	Release(ctx context.Context, req *models.ReleaseRequest) (*models.ReleaseResponse, error)
}

type inventoryService struct {
	repo    repository.InventoryRepository
	log     *logger.Logger
	metrics *metrics.InventoryMetrics
	rdb     *redisclient.Client
}

func New(repo repository.InventoryRepository, log *logger.Logger, m *metrics.InventoryMetrics, rdb *redisclient.Client) InventoryService {
	return &inventoryService{repo: repo, log: log, metrics: m, rdb: rdb}
}

func (s *inventoryService) GetItem(ctx context.Context, itemID string) (*models.InventoryItem, error) {
	tracer := otel.Tracer("inventory-service")
	ctx, span := tracer.Start(ctx, "GetInventoryItem", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(attribute.String("item_id", itemID))
	traceID := middleware.TraceIDFromCtx(ctx)

	if itemID == "" {
		return nil, apperrors.ErrInventoryValidation.WithMessage("item_id is required")
	}

	// 1. Query Redis Caching (Part 9)
	cacheKey := fmt.Sprintf("cache:inventory:%s", itemID)
	cachedVal, err := s.rdb.Get(ctx, "inventory_cache", cacheKey)
	if err == nil && cachedVal != "" {
		var item models.InventoryItem
		if err := json.Unmarshal([]byte(cachedVal), &item); err == nil {
			s.log.Info("returning cached inventory details", "item_id", itemID)
			span.SetAttributes(attribute.Bool("cache_hit", true))
			return &item, nil
		}
	}

	// Cache miss: read from DB
	item, err := s.repo.GetItem(itemID)
	if err != nil {
		s.log.Warn("get item failed",
			"trace_id", traceID,
			"item_id", itemID,
			"error", err.Error(),
		)
		span.RecordError(err)
		return nil, err
	}

	// Set availability gauge
	if s.metrics != nil {
		s.metrics.InventoryAvailable.WithLabelValues(itemID).Set(float64(item.Quantity - item.Reserved))
	}

	// Write back to cache
	itemBytes, err := json.Marshal(item)
	if err == nil {
		_ = s.rdb.Set(ctx, cacheKey, string(itemBytes), 5*time.Minute)
	}

	return item, nil
}

func (s *inventoryService) Reserve(ctx context.Context, req *models.ReserveRequest) (*models.ReserveResponse, error) {
	tracer := otel.Tracer("inventory-service")
	ctx, span := tracer.Start(ctx, "ReserveStock", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("item_id", req.ItemID),
		attribute.Int("quantity", req.Quantity),
		attribute.String("order_id", req.OrderID),
	)

	traceID := middleware.TraceIDFromCtx(ctx)

	if req.ItemID == "" {
		return nil, apperrors.ErrInventoryValidation.WithMessage("item_id is required")
	}
	if req.OrderID == "" {
		return nil, apperrors.ErrInventoryValidation.WithMessage("order_id is required")
	}
	if req.Quantity <= 0 {
		return nil, apperrors.ErrInventoryValidation.WithMessage(
			fmt.Sprintf("quantity must be positive, got %d", req.Quantity),
		)
	}

	// Reserve stock
	if err := s.repo.Reserve(req.ItemID, req.Quantity); err != nil {
		s.log.Warn("inventory reservation failed",
			"trace_id", traceID,
			"item_id", req.ItemID,
			"order_id", req.OrderID,
			"quantity", req.Quantity,
			"error", err.Error(),
		)
		span.RecordError(err)
		return nil, err
	}

	item, err := s.repo.GetItem(req.ItemID)
	if err != nil {
		return nil, err
	}

	available := item.Quantity - item.Reserved
	if s.metrics != nil {
		s.metrics.InventoryReservedTotal.WithLabelValues(req.ItemID).Inc()
		s.metrics.InventoryAvailable.WithLabelValues(req.ItemID).Set(float64(available))
	}

	s.log.Info("inventory reserved",
		"trace_id", traceID,
		"item_id", req.ItemID,
		"order_id", req.OrderID,
		"quantity", req.Quantity,
		"remaining", available,
	)

	// Invalidate Cache since quantity changed
	cacheKey := fmt.Sprintf("cache:inventory:%s", req.ItemID)
	_ = s.rdb.Delete(ctx, cacheKey)

	return &models.ReserveResponse{
		ItemID:       req.ItemID,
		OrderID:      req.OrderID,
		ReservedQty:  req.Quantity,
		RemainingQty: available,
		ReservedAt:   time.Now().UTC(),
	}, nil
}

func (s *inventoryService) Release(ctx context.Context, req *models.ReleaseRequest) (*models.ReleaseResponse, error) {
	tracer := otel.Tracer("inventory-service")
	ctx, span := tracer.Start(ctx, "ReleaseStock", oteltrace.WithSpanKind(oteltrace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(
		attribute.String("item_id", req.ItemID),
		attribute.Int("quantity", req.Quantity),
		attribute.String("order_id", req.OrderID),
	)

	traceID := middleware.TraceIDFromCtx(ctx)

	if req.ItemID == "" {
		return nil, apperrors.ErrInventoryValidation.WithMessage("item_id is required")
	}
	if req.OrderID == "" {
		return nil, apperrors.ErrInventoryValidation.WithMessage("order_id is required")
	}
	if req.Quantity <= 0 {
		return nil, apperrors.ErrInventoryValidation.WithMessage("quantity must be positive")
	}

	// Release stock
	if err := s.repo.Release(req.ItemID, req.Quantity); err != nil {
		s.log.Warn("inventory release failed",
			"trace_id", traceID,
			"item_id", req.ItemID,
			"order_id", req.OrderID,
			"error", err.Error(),
		)
		span.RecordError(err)
		return nil, err
	}

	item, err := s.repo.GetItem(req.ItemID)
	if err != nil {
		return nil, err
	}

	available := item.Quantity - item.Reserved
	if s.metrics != nil {
		s.metrics.InventoryReleasedTotal.WithLabelValues(req.ItemID).Inc()
		s.metrics.InventoryAvailable.WithLabelValues(req.ItemID).Set(float64(available))
	}

	s.log.Info("inventory released",
		"trace_id", traceID,
		"item_id", req.ItemID,
		"order_id", req.OrderID,
		"quantity", req.Quantity,
	)

	// Invalidate Cache since quantity changed
	cacheKey := fmt.Sprintf("cache:inventory:%s", req.ItemID)
	_ = s.rdb.Delete(ctx, cacheKey)

	return &models.ReleaseResponse{
		ItemID:       req.ItemID,
		OrderID:      req.OrderID,
		ReleasedQty:  req.Quantity,
		AvailableQty: available,
		ReleasedAt:   time.Now().UTC(),
	}, nil
}
