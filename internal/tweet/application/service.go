package application

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"UalaTwitter/internal/tweet/domain"
	"UalaTwitter/pkg/apperr"
	"UalaTwitter/pkg/tweetid"
)

const maxChars = 280

type TweetService struct {
	repo domain.TweetRepository
}

func NewTweetService(repo domain.TweetRepository) *TweetService {
	return &TweetService{repo: repo}
}

func (s *TweetService) PostTweet(ctx context.Context, userID, text string, media []domain.MediaItem) (*domain.Tweet, error) {
	if text == "" {
		return nil, apperr.Invalid("tweet text cannot be empty")
	}
	if utf8.RuneCountInString(text) > maxChars {
		return nil, apperr.Invalid(fmt.Sprintf("text exceeds %d characters", maxChars))
	}
	now := time.Now().UTC()
	tweet := &domain.Tweet{
		ID:        tweetid.Compose(userID, now),
		UserID:    userID,
		Text:      text,
		Media:     media,
		CreatedAt: now,
	}
	if err := s.repo.Save(ctx, tweet); err != nil {
		return nil, err
	}
	return tweet, nil
}

func (s *TweetService) GetUserTweets(ctx context.Context, userID string, limit int, beforeID string) ([]*domain.Tweet, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.repo.GetByUserID(ctx, userID, limit, beforeID)
}
