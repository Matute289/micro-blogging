package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"UalaTwitter/internal/timeline/application"
	"UalaTwitter/pkg/httputil"
	"github.com/go-chi/chi/v5"
)

type TimelineHandler struct {
	svc *application.TimelineService
}

func NewTimelineHandler(svc *application.TimelineService) *TimelineHandler {
	return &TimelineHandler{svc: svc}
}

func (h *TimelineHandler) RegisterRoutes(r chi.Router) {
	r.Get("/timeline", h.Get)
}

func (h *TimelineHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing X-User-ID header", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tweets, err := h.svc.GetTimeline(r.Context(), userID, limit, r.URL.Query().Get("before"))
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tweets)
}
