// services/inventory-service/handler/handler.go
//
// Updated to use AppError + WriteAppError, TraceID header, and rich health responses.

package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rohithsandiri/Summer_Project/internal/shared/apperrors"
	"github.com/rohithsandiri/Summer_Project/internal/shared/health"
	"github.com/rohithsandiri/Summer_Project/internal/shared/middleware"
	"github.com/rohithsandiri/Summer_Project/internal/shared/response"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/models"
	"github.com/rohithsandiri/Summer_Project/services/inventory-service/service"
)

// InventoryHandler serves inventory HTTP requests.
type InventoryHandler struct {
	svc    service.InventoryService
	health *health.Handler
}

// New creates an InventoryHandler.
func New(svc service.InventoryService, healthHandler *health.Handler) *InventoryHandler {
	return &InventoryHandler{svc: svc, health: healthHandler}
}

// GetItem handles GET /inventory/{item}
func (h *InventoryHandler) GetItem(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	itemID := strings.TrimPrefix(r.URL.Path, "/inventory/")
	if itemID == "" {
		ae := apperrors.ErrInventoryValidation.WithMessage("item_id is required in path")
		response.WriteAppError(w, traceID, ae)
		return
	}

	item, err := h.svc.GetItem(r.Context(), itemID)
	if err != nil {
		if ae := apperrors.AsAppError(err); ae != nil {
			response.WriteAppError(w, traceID, ae)
			return
		}
		response.WriteAppError(w, traceID, apperrors.ErrInternal.Wrap(err))
		return
	}

	response.Success(w, http.StatusOK, traceID, item)
}

// Reserve handles POST /inventory/reserve
func (h *InventoryHandler) Reserve(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	var req models.ReserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ae := apperrors.ErrInventoryValidation.WithMessage("request body must be valid JSON: " + err.Error())
		response.WriteAppError(w, traceID, ae)
		return
	}

	resp, err := h.svc.Reserve(r.Context(), &req)
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

// Release handles POST /inventory/release
func (h *InventoryHandler) Release(w http.ResponseWriter, r *http.Request) {
	traceID := middleware.TraceIDFromContext(r)

	var req models.ReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ae := apperrors.ErrInventoryValidation.WithMessage("request body must be valid JSON: " + err.Error())
		response.WriteAppError(w, traceID, ae)
		return
	}

	resp, err := h.svc.Release(r.Context(), &req)
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

// Health, Live, Ready delegate to the shared health.Handler.
func (h *InventoryHandler) Health(w http.ResponseWriter, r *http.Request) { h.health.Health(w, r) }
func (h *InventoryHandler) Live(w http.ResponseWriter, r *http.Request)   { h.health.Live(w, r) }
func (h *InventoryHandler) Ready(w http.ResponseWriter, r *http.Request)  { h.health.Ready(w, r) }
