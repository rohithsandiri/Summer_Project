// services/order-service/handler/handler.go
//
// Updated: uses WriteAppError, TraceID, delegates health probes.

package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/internal/shared/health"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/response"
	"github.com/rohithsandiri/Summer_Project/services/order-service/models"
	"github.com/rohithsandiri/Summer_Project/services/order-service/service"
)

// OrderHandler serves order HTTP requests.
type OrderHandler struct {
	svc    service.OrderService
	health *health.Handler
}

// New creates an OrderHandler.
func New(svc service.OrderService, healthHandler *health.Handler) *OrderHandler {
	return &OrderHandler{svc: svc, health: healthHandler}
}

// CreateOrder handles POST /orders
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	var req models.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ae := apperrors.ErrOrderValidation.WithMessage("request body must be valid JSON: " + err.Error())
		response.WriteAppError(w, traceID, ae)
		return
	}

	resp, err := h.svc.CreateOrder(r.Context(), traceID, &req)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	response.Success(w, http.StatusCreated, traceID, resp)
}

// GetOrder handles GET /orders/{id}
func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	orderID := strings.TrimPrefix(r.URL.Path, "/orders/")
	if orderID == "" {
		ae := apperrors.ErrOrderValidation.WithMessage("order_id is required in path")
		response.WriteAppError(w, traceID, ae)
		return
	}

	order, err := h.svc.GetOrder(r.Context(), orderID)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	response.Success(w, http.StatusOK, traceID, order)
}

// Health, Live, Ready delegate to shared health.Handler.
func (h *OrderHandler) Health(w http.ResponseWriter, r *http.Request) { h.health.Health(w, r) }
func (h *OrderHandler) Live(w http.ResponseWriter, r *http.Request)   { h.health.Live(w, r) }
func (h *OrderHandler) Ready(w http.ResponseWriter, r *http.Request)  { h.health.Ready(w, r) }
