// internal/operator/webhook/handler.go
//
// HTTP Handler for receiving Alertmanager webhook invocations.

package webhook

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rohithsandiri/Summer_Project/internal/operator/alert"
	"github.com/rohithsandiri/Summer_Project/internal/operator/incident"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type Handler struct {
	parser  *alert.Parser
	manager *incident.Manager
	log     *logger.Logger
}

func NewHandler(parser *alert.Parser, manager *incident.Manager, log *logger.Logger) *Handler {
	return &Handler{
		parser:  parser,
		manager: manager,
		log:     log,
	}
}

// ServeHTTP parses, validates, and processes incoming Alertmanager alert webhooks.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = r.Header.Get("X-Correlation-ID")
	}
	if traceID == "" {
		traceID = fmt.Sprintf("tr-%x", h.randomBytes(8))
	}

	f := logger.Fields{
		TraceID: traceID,
	}

	if r.Method != http.MethodPost {
		h.log.Warn(ctx, "received invalid request method", f, "method", r.Method)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload models.AlertmanagerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.log.Error(ctx, "failed to decode json body", f, "error", err.Error())
		http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
		return
	}

	alerts, err := h.parser.Parse(&payload)
	if err != nil {
		h.log.Error(ctx, "failed to parse/validate alertmanager payload", f, "error", err.Error())
		http.Error(w, fmt.Sprintf("Bad Request: %v", err), http.StatusBadRequest)
		return
	}

	h.log.Info(ctx, "received valid alertmanager payload", f, "alerts_count", len(alerts))

	// Process alerts asynchronously to return 200 OK fast (Alertmanager requirement)
	for _, a := range alerts {
		go func(alertItem *models.Alert) {
			if err := h.manager.ProcessAlert(ctx, alertItem, traceID); err != nil {
				h.log.Error(ctx, "error processing alert", logger.Fields{
					TraceID:   traceID,
					AlertName: alertItem.Name,
					Service:   alertItem.Service,
					Reason:    err.Error(),
				})
			}
		}(a)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

func (h *Handler) randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
