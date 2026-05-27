package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"aggregator/internal/domain"
)

type APIHandlers struct {
	eventRepo   domain.EventRepository
	summaryRepo domain.SummaryRepository
	logger      *slog.Logger
}

func NewAPIHandlers(
	eventRepo domain.EventRepository,
	summaryRepo domain.SummaryRepository,
	logger *slog.Logger,
) *APIHandlers {
	return &APIHandlers{
		eventRepo:   eventRepo,
		summaryRepo: summaryRepo,
		logger:      logger,
	}
}

// Health godoc
// @Summary      Liveness probe
// @Description  Returns 200 when the aggregator process is running
// @Tags         ops
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /health [get]
func (h *APIHandlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"service": "aggregator",
		"time":    time.Now().UTC(),
	})
}

// GetDeveloperMetrics godoc
// @Summary      List developer events
// @Description  Returns every processed event stored for the given developer
// @Tags         metrics
// @Produce      json
// @Param        developer_id  path      string  true  "Developer ID"
// @Success      200           {object}  map[string]interface{}
// @Failure      400           {string}  string  "developer_id is required"
// @Failure      500           {string}  string  "internal error"
// @Router       /metrics/{developer_id} [get]
func (h *APIHandlers) GetDeveloperMetrics(w http.ResponseWriter, r *http.Request) {
	developerID := chi.URLParam(r, "developer_id")
	if developerID == "" {
		http.Error(w, "developer_id is required", http.StatusBadRequest)
		return
	}

	events, err := h.eventRepo.GetByDeveloperID(r.Context(), developerID)
	if err != nil {
		h.logger.Error("fetching events", "developer_id", developerID, "error", err)
		http.Error(w, fmt.Sprintf("error fetching events: %v", err), http.StatusInternalServerError)
		return
	}

	if events == nil {
		events = []*domain.ProcessedEvent{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"developer_id": developerID,
		"events":       events,
		"count":        len(events),
	})
}

// GetDeveloperSummary godoc
// @Summary      Developer summary
// @Description  Returns aggregated totals (commits, PRs, review time) for the developer
// @Tags         metrics
// @Produce      json
// @Param        developer_id  path      string                  true  "Developer ID"
// @Success      200           {object}  domain.DeveloperSummary
// @Failure      400           {string}  string                  "developer_id is required"
// @Failure      500           {string}  string                  "internal error"
// @Router       /metrics/{developer_id}/summary [get]
func (h *APIHandlers) GetDeveloperSummary(w http.ResponseWriter, r *http.Request) {
	developerID := chi.URLParam(r, "developer_id")
	if developerID == "" {
		http.Error(w, "developer_id is required", http.StatusBadRequest)
		return
	}

	summary, err := h.summaryRepo.GetOrCreate(r.Context(), developerID)
	if err != nil {
		h.logger.Error("fetching summary", "developer_id", developerID, "error", err)
		http.Error(w, fmt.Sprintf("error fetching summary: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(summary)
}

func (h *APIHandlers) RegisterRoutes(router *chi.Mux) {
	router.Get("/health", h.Health)
	router.Get("/metrics/{developer_id}", h.GetDeveloperMetrics)
	router.Get("/metrics/{developer_id}/summary", h.GetDeveloperSummary)
}