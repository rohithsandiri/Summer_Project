// internal/operator/reliability/handler.go

package reliability

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rohithsandiri/Summer_Project/internal/operator/interfaces"
	"github.com/rohithsandiri/Summer_Project/internal/operator/logger"
	"github.com/rohithsandiri/Summer_Project/internal/operator/models"
)

type ReliabilityHandler struct {
	store       interfaces.IncidentStore
	timeline    TimelineEngine
	reliability *ReliabilityEngine
	log         *logger.Logger
}

func NewReliabilityHandler(
	store interfaces.IncidentStore,
	timeline TimelineEngine,
	reliability *ReliabilityEngine,
	log *logger.Logger,
) *ReliabilityHandler {
	return &ReliabilityHandler{
		store:       store,
		timeline:    timeline,
		reliability: reliability,
		log:         log,
	}
}

func (h *ReliabilityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/incidents", h.HandleIncidents)
	mux.HandleFunc("/incidents/active", h.HandleActiveIncidents)
	mux.HandleFunc("/incidents/history", h.HandleHistoricalIncidents)
	mux.HandleFunc("/analytics", h.HandleAnalytics)
	mux.HandleFunc("/runbooks", h.HandleRunbooks)
	mux.HandleFunc("/reliability", h.HandleReliability)
	mux.HandleFunc("/recommendations", h.HandleRecommendations)
	mux.HandleFunc("/timelines", h.HandleTimelines)
}

func (h *ReliabilityHandler) HandleIncidents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":"Method not allowed"}`))
		return
	}

	id := r.URL.Query().Get("id")
	if id != "" {
		inc, err := h.store.Get(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Incident not found"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(inc)
		return
	}

	incidents, err := h.store.ListAll(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Failed to retrieve incidents"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(incidents)
}

func (h *ReliabilityHandler) HandleActiveIncidents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	incidents, err := h.store.ListActive(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(incidents)
}

func (h *ReliabilityHandler) HandleHistoricalIncidents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	incidents, err := h.store.ListAll(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var resolved []*models.Incident
	for _, inc := range incidents {
		if inc.Status == "resolved" {
			resolved = append(resolved, inc)
		}
	}
	_ = json.NewEncoder(w).Encode(resolved)
}

func (h *ReliabilityHandler) HandleAnalytics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	scope := "cluster"
	target := ""
	if service != "" {
		scope = "service"
		target = service
	}

	mttrReport, err := h.reliability.CalculateMTTR(r.Context(), scope, target)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	relReport, err := h.reliability.CalculateReliability(r.Context(), scope, target)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"mttr":        mttrReport,
		"reliability": relReport,
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (h *ReliabilityHandler) HandleRunbooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	incidentID := r.URL.Query().Get("incident_id")
	if incidentID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"incident_id query parameter is required"}`))
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	rb, err := h.reliability.GenerateRunbook(r.Context(), incidentID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Incident not found"}`))
		return
	}

	formatted, err := h.reliability.FormatRunbook(rb, format)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if strings.ToLower(format) == "html" {
		w.Header().Set("Content-Type", "text/html")
	} else if strings.ToLower(format) == "markdown" || strings.ToLower(format) == "md" {
		w.Header().Set("Content-Type", "text/plain")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(formatted))
}

func (h *ReliabilityHandler) HandleReliability(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	scope := "cluster"
	target := ""
	if service != "" {
		scope = "service"
		target = service
	}

	report, err := h.reliability.CalculateReliability(r.Context(), scope, target)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(report)
}

func (h *ReliabilityHandler) HandleRecommendations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"service query parameter is required"}`))
		return
	}

	recs, err := h.reliability.GetRecommendations(r.Context(), service)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(recs)
}

func (h *ReliabilityHandler) HandleTimelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	incidentID := r.URL.Query().Get("incident_id")
	if incidentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"incident_id query parameter is required"}`))
		return
	}

	timeline, err := h.timeline.GetTimeline(r.Context(), incidentID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(timeline)
}
