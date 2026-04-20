package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"UalaTwitter/internal/tweet/application"
	"UalaTwitter/internal/tweet/domain"
	"UalaTwitter/pkg/httputil"
	"github.com/go-chi/chi/v5"
)

// TimelineFanOut is satisfied by timeline.TimelineService.
type TimelineFanOut interface {
	FanOutTweet(ctx context.Context, posterID, tweetID string, createdAt time.Time)
}

type TweetHandler struct {
	svc     *application.TweetService
	fanOut  TimelineFanOut
}

func NewTweetHandler(svc *application.TweetService, fanOut TimelineFanOut) *TweetHandler {
	return &TweetHandler{svc: svc, fanOut: fanOut}
}

func (h *TweetHandler) RegisterRoutes(r chi.Router) {
	r.Post("/tweets", h.Post)
	r.Get("/users/{id}/tweets", h.ListByUser)
}

func (h *TweetHandler) Post(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "missing X-User-ID header", http.StatusBadRequest)
		return
	}
	var body struct {
		Text  string           `json:"text"`
		Media []domain.MediaItem `json:"media"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	tweet, err := h.svc.PostTweet(r.Context(), userID, body.Text, body.Media)
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	go h.fanOut.FanOutTweet(context.Background(), userID, tweet.ID, tweet.CreatedAt)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tweet)
}

func (h *TweetHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	tweets, err := h.svc.GetUserTweets(r.Context(), chi.URLParam(r, "id"), limit, r.URL.Query().Get("before"))
	if err != nil {
		httputil.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tweets)
}
