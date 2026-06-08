package application_test

import (
	"context"
	"fmt"
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

func (m *mockFollowRepo) Follow(_ context.Context, _, _ string) error   { return nil }
func (m *mockFollowRepo) Unfollow(_ context.Context, _, _ string) error { return nil }
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
func (m *mockTweetRepo) GetByID(_ context.Context, id string) (*domain.Tweet, error) {
	for _, t := range m.tweets {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, fmt.Errorf("tweet not found: %s", id)
}
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

type mockNotifier struct {
	calls []struct {
		userID string
		tweet  *domain.Tweet
	}
}

func (m *mockNotifier) Notify(_ context.Context, userID string, tweet *domain.Tweet) {
	m.calls = append(m.calls, struct {
		userID string
		tweet  *domain.Tweet
	}{userID, tweet})
}

// --- helpers ---

func makeTweet(id, userID string) *domain.Tweet {
	return &domain.Tweet{ID: id, UserID: userID, Text: "hello", CreatedAt: time.Now()}
}

// --- GetTimeline tests ---

func TestGetTimeline_CacheMiss_QueriesMongo(t *testing.T) {
	followRepo := &mockFollowRepo{following: []string{"user-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("tweet-1", "user-2")}}
	cache := newMockCache(false)

	svc := application.NewTimelineService(followRepo, tweetRepo, cache, application.NopNotifier{})
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

	svc := application.NewTimelineService(followRepo, tweetRepo, cache, application.NopNotifier{})
	_, _ = svc.GetTimeline(context.Background(), "user-1", 20, "")

	if len(cache.data["user-1"]) != 1 {
		t.Error("expected cache to be populated after miss")
	}
}

func TestGetTimeline_CacheHit_SkipsMongo(t *testing.T) {
	followRepo := &mockFollowRepo{}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{makeTweet("tweet-1", "user-2")}}
	cache := newMockCache(true)
	cache.data["user-1"] = []string{"tweet-1"}

	svc := application.NewTimelineService(followRepo, tweetRepo, cache, application.NopNotifier{})
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
	svc := application.NewTimelineService(followRepo, &mockTweetRepo{}, newMockCache(false), application.NopNotifier{})

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

	svc := application.NewTimelineService(followRepo, tweetRepo, newMockCache(false), application.NopNotifier{})
	_, err := svc.GetTimeline(context.Background(), "user-1", 0, "")
	if err != nil {
		t.Fatalf("unexpected error with limit=0: %v", err)
	}
}

// --- FanOutTweet tests ---

func TestFanOutTweet_OnlyPushesToWarmCaches(t *testing.T) {
	tweet := makeTweet("tweet-1", "poster")
	followRepo := &mockFollowRepo{followers: []string{"follower-1", "follower-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{tweet}}
	cache := newMockCache(false)

	svc := application.NewTimelineService(followRepo, tweetRepo, cache, application.NopNotifier{})
	if err := svc.FanOutTweet(context.Background(), "poster", tweet.ID, tweet.CreatedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.data) != 0 {
		t.Error("fan-out should not push to cold caches")
	}
}

func TestFanOutTweet_PushesToWarmCache(t *testing.T) {
	tweet := makeTweet("tweet-1", "poster")
	followRepo := &mockFollowRepo{followers: []string{"follower-1"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{tweet}}
	cache := newMockCache(true)

	svc := application.NewTimelineService(followRepo, tweetRepo, cache, application.NopNotifier{})
	if err := svc.FanOutTweet(context.Background(), "poster", tweet.ID, tweet.CreatedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cache.data["follower-1"]) != 1 {
		t.Error("expected warm cache to be updated")
	}
}

func TestFanOutTweet_NotifiesAllFollowers(t *testing.T) {
	tweet := makeTweet("tweet-1", "poster")
	followRepo := &mockFollowRepo{followers: []string{"follower-1", "follower-2"}}
	tweetRepo := &mockTweetRepo{tweets: []*domain.Tweet{tweet}}
	notifier := &mockNotifier{}

	svc := application.NewTimelineService(followRepo, tweetRepo, newMockCache(false), notifier)
	if err := svc.FanOutTweet(context.Background(), "poster", tweet.ID, tweet.CreatedAt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(notifier.calls) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifier.calls))
	}
	if notifier.calls[0].userID != "follower-1" || notifier.calls[1].userID != "follower-2" {
		t.Errorf("unexpected notification targets: %v", notifier.calls)
	}
	if notifier.calls[0].tweet.ID != tweet.ID {
		t.Errorf("expected tweet id %s, got %s", tweet.ID, notifier.calls[0].tweet.ID)
	}
}

func TestFanOutTweet_NoFollowers_NoNotifications(t *testing.T) {
	followRepo := &mockFollowRepo{followers: []string{}}
	notifier := &mockNotifier{}

	svc := application.NewTimelineService(followRepo, &mockTweetRepo{}, newMockCache(false), notifier)
	if err := svc.FanOutTweet(context.Background(), "poster", "tweet-1", time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Error("expected no notifications with no followers")
	}
}
