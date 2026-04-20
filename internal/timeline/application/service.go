package application

import (
	"context"
	"fmt"
	"time"

	followdomain "UalaTwitter/internal/follow/domain"
	tweetdomain "UalaTwitter/internal/tweet/domain"
	"UalaTwitter/pkg/tweetid"
)

type TimelineCache interface {
	Push(ctx context.Context, userID, tweetID string, score float64) error
	Range(ctx context.Context, userID string, limit int, max string) ([]string, error)
	Exists(ctx context.Context, userID string) (bool, error)
}

type TimelineService struct {
	followRepo followdomain.FollowRepository
	tweetRepo  tweetdomain.TweetRepository
	cache      TimelineCache
}

func NewTimelineService(
	followRepo followdomain.FollowRepository,
	tweetRepo tweetdomain.TweetRepository,
	cache TimelineCache,
) *TimelineService {
	return &TimelineService{followRepo: followRepo, tweetRepo: tweetRepo, cache: cache}
}

// FanOutTweet pushes a new tweet ID to the Redis timeline of each follower that has a warm cache.
// Called asynchronously from the tweet handler after a successful post.
func (s *TimelineService) FanOutTweet(ctx context.Context, posterID, tweetID string, createdAt time.Time) {
	followers, err := s.followRepo.GetFollowers(ctx, posterID)
	if err != nil || len(followers) == 0 {
		return
	}
	score := float64(createdAt.UnixMilli())
	for _, followerID := range followers {
		exists, err := s.cache.Exists(ctx, followerID)
		if err != nil || !exists {
			continue // only push to warm caches; cold caches rebuild on first read
		}
		_ = s.cache.Push(ctx, followerID, tweetID, score)
	}
}

// GetTimeline returns the caller's timeline, newest first, with cursor-based pagination.
// It reads from Redis when available; on cache miss it falls back to Postgres + MongoDB.
func (s *TimelineService) GetTimeline(ctx context.Context, userID string, limit int, beforeID string) ([]*tweetdomain.Tweet, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	max := "+inf"
	if beforeID != "" {
		if _, t, err := tweetid.Decompose(beforeID); err == nil {
			max = fmt.Sprintf("(%d", t.UnixMilli()) // exclusive upper bound
		}
	}

	if exists, _ := s.cache.Exists(ctx, userID); exists {
		ids, err := s.cache.Range(ctx, userID, limit, max)
		if err == nil && len(ids) > 0 {
			return s.tweetRepo.GetByIDs(ctx, ids)
		}
	}

	// Cache miss: build from source
	following, err := s.followRepo.GetFollowing(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(following) == 0 {
		return []*tweetdomain.Tweet{}, nil
	}
	tweets, err := s.tweetRepo.GetByUserIDs(ctx, following, limit, beforeID)
	if err != nil {
		return nil, err
	}
	// Warm the cache for subsequent requests
	for _, t := range tweets {
		_ = s.cache.Push(ctx, userID, t.ID, float64(t.CreatedAt.UnixMilli()))
	}
	return tweets, nil
}
