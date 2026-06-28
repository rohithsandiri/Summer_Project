// services/payment-service/handler/handler.go
//
// Updated to use WriteAppError, TraceID, and rich health responses.

package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/internal/shared/health"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/response"
	"github.com/rohithsandiri/Summer_Project/services/payment-service/models"
	"github.com/rohithsandiri/Summer_Project/services/payment-service/service"
)

// PaymentHandler serves payment HTTP requests.
type PaymentHandler struct {
	svc    service.PaymentService
	health *health.Handler
}

// New creates a PaymentHandler.
func New(svc service.PaymentService, healthHandler *health.Handler) *PaymentHandler {
	return &PaymentHandler{svc: svc, health: healthHandler}
}

// Process handles POST /payment/process
func (h *PaymentHandler) Process(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	var req models.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ae := apperrors.ErrPaymentValidation.WithMessage("request body must be valid JSON: " + err.Error())
		response.WriteAppError(w, traceID, ae)
		return
	}

	resp, err := h.svc.Process(r.Context(), &req)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	// Always 200 — the payment gateway outcome is in resp.Status.
	// HTTP 500 would mean our service crashed, not that the card was declined.
	response.Success(w, http.StatusOK, traceID, resp)
}

// Refund handles POST /payment/refund
func (h *PaymentHandler) Refund(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	var req models.RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ae := apperrors.ErrPaymentValidation.WithMessage("request body must be valid JSON: " + err.Error())
		response.WriteAppError(w, traceID, ae)
		return
	}

	resp, err := h.svc.Refund(r.Context(), &req)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	response.Success(w, http.StatusOK, traceID, resp)
}

// GetPayment handles GET /payment/{id}
func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	paymentID := strings.TrimPrefix(r.URL.Path, "/payment/")
	if paymentID == "" {
		ae := apperrors.ErrPaymentValidation.WithMessage("payment_id is required in path")
		response.WriteAppError(w, traceID, ae)
		return
	}

	payment, err := h.svc.GetPayment(r.Context(), paymentID)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	response.Success(w, http.StatusOK, traceID, payment)
}

// Health, Live, Ready delegate to shared health.Handler.
func (h *PaymentHandler) Health(w http.ResponseWriter, r *http.Request) { h.health.Health(w, r) }
func (h *PaymentHandler) Live(w http.ResponseWriter, r *http.Request)   { h.health.Live(w, r) }
func (h *PaymentHandler) Ready(w http.ResponseWriter, r *http.Request)  { h.health.Ready(w, r) }
