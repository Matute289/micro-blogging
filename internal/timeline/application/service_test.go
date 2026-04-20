package application_test

import (
	"context"
	"testing"
	"time"

	"UalaTwitter/internal/timeline/application"
	"UalaTwitter/internal/tweet/domain"
)

// --- mocks ---

type mockFollowRepo struct {
	following []string
	followers []string
}

func (m *mockFollowRepo) Follow(_ context.Context, _, _ string) error        { return nil }
func (m *mockFollowRepo) Unfollow(_ context.Context, _, _ string) error      { return nil }
func (m *mockFollowRepo) GetFollowing(_ context.Context, _ string) ([]string, error) {
	return m.following, nil
}
func (m *mockFollowRepo) GetFollowers(_ context.Context, _ string) ([]string, error) {
	return m.followers, nil
}

type mockTweetRepo struct {
	tweets []*domain.Tweet
}

func (m *mockTweetRepo) Save(_ context.Context, _ *domain.Tweet) error { return nil }
func (m *mockTweetRepo) GetByUserID(_ context.Context, _ string, _ int, _ string) ([]*domain.Tweet, error) {
	return m.tweets, nil
}
func (m *mockTweetRepo) GetByUserIDs(_ context.Context, _ []string, _ int, _ string) ([]*domain.Tweet, error) {
	return m.tweets, nil
}
func (m *mockTweetRepo) GetByIDs(_ context.Context, _ []string) ([]*domain.Tweet, error) {
	return m.tweets, nil
}

type mockCache struct {
	data   map[string][]string
	exists bool
}

func newMockCache(exists bool) *mockCache {
	return &mockCache{data: make(map[string][]string), exists: exists}
}

func (m *mockCache) Push(_ context.Context, userID, tweetID string, _ float64) error {
	m.data[userID] = append(m.data[userID], tweetID)
	return nil
}
func (m *mockCache) Range(_ context.Context, userID string, _ int, _ string) ([]string, error) {
	return m.data[userID], nil
}
func (m *mockCache) Exists(_ context.Context, _ string) (bool, error) {
	return m.exists, nil
}

// --- tests ---

func makeTweet(id, userID string) *domain.Tweet {
	return &domain.Tweet{ID: id, UserID: userID, Text: "hello", CreatedAt: time.Now()}
}

func TestGetTimeline_CacheMiss_QueriesMongo(t *testing.T) {
	followRepo := &mockFollowRepo{following: []string{"user-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("tweet-1", "user-2")}}
	cache := newMockCache(false)

	svc := application.NewTimelineService(followRepo, tweetRepo, cache)
	result, err := svc.GetTimeline(context.Background(), "user-1", 20, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 tweet, got %d", len(result))
	}
}

func TestGetTimeline_CacheMiss_PopulatesCache(t *testing.T) {
	followRepo := &mockFollowRepo{following: []string{"user-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("tweet-1", "user-2")}}
	cache := newMockCache(false)

	svc := application.NewTimelineService(followRepo, tweetRepo, cache)
	_, _ = svc.GetTimeline(context.Background(), "user-1", 20, "")

	if len(cache.data["user-1"]) != 1 {
		t.Error("expected cache to be populated after miss")
	}
}

func TestGetTimeline_CacheHit_SkipsMongo(t *testing.T) {
	followRepo := &mockFollowRepo{}
	// tweetRepo returns these tweets when GetByIDs is called (cache hit path)
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("tweet-1", "user-2")}}
	cache := newMockCache(true)
	cache.data["user-1"] = []string{"tweet-1"}

	svc := application.NewTimelineService(followRepo, tweetRepo, cache)
	result, err := svc.GetTimeline(context.Background(), "user-1", 20, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 tweet from cache, got %d", len(result))
	}
}

func TestGetTimeline_NoFollowing_ReturnsEmpty(t *testing.T) {
	followRepo := &mockFollowRepo{following: []string{}}
	svc := application.NewTimelineService(followRepo, &mockTweetRepo{}, newMockCache(false))

	result, err := svc.GetTimeline(context.Background(), "user-1", 20, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty timeline, got %d tweets", len(result))
	}
}

func TestGetTimeline_LimitClamped(t *testing.T) {
	followRepo := &mockFollowRepo{following: []string{"user-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("t1", "user-2")}}

	svc := application.NewTimelineService(followRepo, tweetRepo, newMockCache(false))
	// limit=0 should be clamped to 20 internally - just verify it doesn't error
	_, err := svc.GetTimeline(context.Background(), "user-1", 0, "")
	if err != nil {
		t.Fatalf("unexpected error with limit=0: %v", err)
	}
}

func TestFanOutTweet_OnlyPushesToWarmCaches(t *testing.T) {
	followRepo := &mockFollowRepo{followers: []string{"follower-1", "follower-2"}}
	cache := newMockCache(false) // no cache is warm

	svc := application.NewTimelineService(followRepo, &mockTweetRepo{}, cache)
	svc.FanOutTweet(context.Background(), "poster", "tweet-1", time.Now())

	if len(cache.data) != 0 {
		t.Error("fan-out should not push to cold caches")
	}
}
