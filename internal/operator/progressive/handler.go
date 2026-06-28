// internal/operator/progressive/handler.go

package progressive

import (
	"encoding/json"
	"net/http"
	"strings"
)

type ProgressiveHandler struct {
	manager ProgressiveDeliveryManager
	riskEng DeploymentRiskEngine
}

func NewProgressiveHandler(mgr ProgressiveDeliveryManager, risk DeploymentRiskEngine) *ProgressiveHandler {
	return &ProgressiveHandler{
		manager: mgr,
		riskEng: risk,
	}
}

func (h *ProgressiveHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/rollouts", h.handleRollouts)
	mux.HandleFunc("/rollouts/", h.handleRolloutDetail)
	mux.HandleFunc("/deployments", h.handleDeployments)
	mux.HandleFunc("/deployments/history", h.handleDeployments)
	mux.HandleFunc("/deployment-risk", h.handleDeploymentRisk)
	mux.HandleFunc("/promotion-history", h.handlePromotionHistory)
	mux.HandleFunc("/canary", h.handleCanary)
	mux.HandleFunc("/analysis", h.handleAnalysis)
}

func (h *ProgressiveHandler) handleRollouts(w http.ResponseWriter, r *http.Request) {
	deploys, err := h.manager.ListDeployments(r.Context())
	if err != nil {
		h.respondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.respondJSON(w, deploys)
}

func (h *ProgressiveHandler) handleRolloutDetail(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 || pathParts[2] == "" {
		h.respondError(w, "missing rollout id", http.StatusBadRequest)
		return
	}
	id := pathParts[2]
	deploy, err := h.manager.GetDeployment(r.Context(), id)
	if err != nil {
		h.respondError(w, err.Error(), http.StatusNotFound)
		return
	}
	h.respondJSON(w, deploy)
}

func (h *ProgressiveHandler) handleDeployments(w http.ResponseWriter, r *http.Request) {
	deploys, err := h.manager.ListDeployments(r.Context())
	if err != nil {
		h.respondError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.respondJSON(w, deploys)
}

func (h *ProgressiveHandler) handleDeploymentRisk(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if service == "" {
		h.respondError(w, "missing service query parameter", http.StatusBadRequest)
		return
	}

	analysis, err := h.riskEng.CalculateRisk(r.Context(), service)
	if err != nil {
		h.respondError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.respondJSON(w, analysis)
}

func (h *ProgressiveHandler) handlePromotionHistory(w http.ResponseWriter, r *http.Request) {
	deploys, err := h.manager.ListDeployments(r.Context())
	if err != nil {
		h.respondError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter only completed (promoted) deployments
	promoted := make([]interface{}, 0)
	for _, d := range deploys {
		if d.PromotionResult == "Promoted" {
			promoted = append(promoted, d)
		}
	}
	h.respondJSON(w, promoted)
}

func (h *ProgressiveHandler) handleCanary(w http.ResponseWriter, r *http.Request) {
	deploys, err := h.manager.ListDeployments(r.Context())
	if err != nil {
		h.respondError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter only active canary run states
	canaries := make([]interface{}, 0)
	for _, d := range deploys {
		if d.CurrentState == "Canary" || d.CurrentState == "Paused" {
			canaries = append(canaries, d)
		}
	}
	h.respondJSON(w, canaries)
}

func (h *ProgressiveHandler) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	// Simple analysis status response
	status := map[string]string{"status": "Analysis engine is healthy and collecting telemetry"}
	h.respondJSON(w, status)
}

func (h *ProgressiveHandler) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *ProgressiveHandler) respondError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
