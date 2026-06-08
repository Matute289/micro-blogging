package handler

import (
	"net/http"

	"UalaTwitter/internal/follow/application"
	"UalaTwitter/pkg/httputil"
	mw "UalaTwitter/pkg/middleware"
	"github.com/go-chi/chi/v5"
)

type FollowHandler struct {
	svc *application.FollowService
}

func NewFollowHandler(svc *application.FollowService) *FollowHandler {
	return &FollowHandler{svc: svc}
}

func (h *FollowHandler) RegisterRoutes(r chi.Router) {
	r.Post("/users/{id}/follow", h.Follow)
	r.Delete("/users/{id}/follow", h.Unfollow)
}

func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	followerID, ok := mw.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.svc.Follow(r.Context(), followerID, chi.URLParam(r, "id")); err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	followerID, ok := mw.UserIDFromCtx(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := h.svc.Unfollow(r.Context(), followerID, chi.URLParam(r, "id")); err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
