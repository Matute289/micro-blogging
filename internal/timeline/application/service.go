package application

import (
	"context"
	"fmt"
	"log/slog"
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

// Notifier is the outbound port for pushing tweets to connected clients (e.g. WebSocket hub).
type Notifier interface {
	Notify(ctx context.Context, userID string, tweet *tweetdomain.Tweet)
}

// NopNotifier is a no-op Notifier for tests that don't need WS push behavior.
type NopNotifier struct{}

func (NopNotifier) Notify(_ context.Context, _ string, _ *tweetdomain.Tweet) {}

type TimelineService struct {
	followRepo followdomain.FollowRepository
	tweetRepo  tweetdomain.TweetRepository
	cache      TimelineCache
	notifier   Notifier
}

func NewTimelineService(
	followRepo followdomain.FollowRepository,
	tweetRepo tweetdomain.TweetRepository,
	cache TimelineCache,
	notifier Notifier,
) *TimelineService {
	return &TimelineService{followRepo: followRepo, tweetRepo: tweetRepo, cache: cache, notifier: notifier}
}

// FanOutTweet pushes a new tweet ID to the Redis timeline of each follower that has a warm cache.
// Returns an error if Postgres or Redis is unavailable so the caller can retry.
func (s *TimelineService) FanOutTweet(ctx context.Context, posterID, tweetID string, createdAt time.Time) error {
	followers, err := s.followRepo.GetFollowers(ctx, posterID)
	if err != nil {
		return err
	}
	if len(followers) == 0 {
		return nil
	}

	tweet, err := s.tweetRepo.GetByID(ctx, tweetID)
	if err != nil {
		// The tweet was just saved, so this should never happen in practice.
		slog.Warn("fanout: failed to fetch tweet for notification", "tweet_id", tweetID, "err", err)
		tweet = nil
	}

	score := float64(createdAt.UnixMilli())
	for _, followerID := range followers {
		exists, err := s.cache.Exists(ctx, followerID)
		if err != nil {
			return err
		}
		if exists {
			if err := s.cache.Push(ctx, followerID, tweetID, score); err != nil {
				return err
			}
		}
		if tweet != nil {
			s.notifier.Notify(ctx, followerID, tweet)
		}
	}
	return nil
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
