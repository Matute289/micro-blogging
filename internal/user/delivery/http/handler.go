package handler

import (
	"encoding/json"
	"net/http"

	"UalaTwitter/internal/user/application"
	"UalaTwitter/pkg/httputil"
	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	svc *application.UserService
}

func NewUserHandler(svc *application.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Post("/users", h.Create)
	r.Get("/users/{id}", h.Get)
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}
	user, err := h.svc.CreateUser(r.Context(), body.Username)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
