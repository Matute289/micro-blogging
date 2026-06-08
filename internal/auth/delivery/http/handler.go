package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"UalaTwitter/internal/auth/application"
	"UalaTwitter/internal/user/domain"
	"UalaTwitter/pkg/httputil"
)

type AuthHandler struct {
	svc *application.AuthService
}

func NewAuthHandler(svc *application.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/auth/google", h.Google)
	r.Post("/auth/github", h.GitHub)
	r.Post("/auth/apple", h.Apple)
}

type authResponse struct {
	Token string       `json:"token"`
	User  *domain.User `json:"user"`
}

func (h *AuthHandler) Google(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IDToken == "" {
		http.Error(w, "missing id_token", http.StatusBadRequest)
		return
	}
	user, err := h.svc.LoginWithGoogle(r.Context(), body.IDToken)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	h.respond(w, r, user)
}

func (h *AuthHandler) GitHub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	user, err := h.svc.LoginWithGitHub(r.Context(), body.Code)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	h.respond(w, r, user)
}

func (h *AuthHandler) Apple(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IDToken == "" {
		http.Error(w, "missing id_token", http.StatusBadRequest)
		return
	}
	user, err := h.svc.LoginWithApple(r.Context(), body.IDToken)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	h.respond(w, r, user)
}

func (h *AuthHandler) respond(w http.ResponseWriter, r *http.Request, user *domain.User) {
	token, err := h.svc.IssueToken(user)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(authResponse{Token: token, User: user})
}
