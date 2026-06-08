package application_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"UalaTwitter/internal/tweet/application"
	"UalaTwitter/internal/tweet/domain"
	"UalaTwitter/pkg/apperr"
)

type mockTweetRepo struct {
	saved *domain.Tweet
}

func (m *mockTweetRepo) Save(ctx context.Context, t *domain.Tweet) error {
	m.saved = t
	return nil
}
func (m *mockTweetRepo) GetByUserID(_ context.Context, _ string, _ int, _ string) ([]*domain.Tweet, error) {
	return nil, nil
}
func (m *mockTweetRepo) GetByUserIDs(_ context.Context, _ []string, _ int, _ string) ([]*domain.Tweet, error) {
	return nil, nil
}
func (m *mockTweetRepo) GetByID(_ context.Context, _ string) (*domain.Tweet, error) {
	return nil, nil
}
func (m *mockTweetRepo) GetByIDs(_ context.Context, _ []string) ([]*domain.Tweet, error) {
	return nil, nil
}

const testUserID = "550e8400-e29b-41d4-a716-446655440000"

func TestPostTweet_Valid(t *testing.T) {
	repo := &mockTweetRepo{}
	svc := application.NewTweetService(repo)

	tweet, err := svc.PostTweet(context.Background(), testUserID, "hello world", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tweet.ID == "" {
		t.Error("expected non-empty tweet ID")
	}
	if tweet.Text != "hello world" {
		t.Errorf("text: got %q, want %q", tweet.Text, "hello world")
	}
	if repo.saved == nil {
		t.Error("expected tweet to be persisted")
	}
}

func TestPostTweet_ExceedsLimit(t *testing.T) {
	svc := application.NewTweetService(&mockTweetRepo{})
	_, err := svc.PostTweet(context.Background(), testUserID, strings.Repeat("a", 281), nil)
	if err == nil {
		t.Fatal("expected error for text exceeding 280 chars")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperr.Error, got %T", err)
	}
	if appErr.Status() != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", appErr.Status(), http.StatusBadRequest)
	}
}

func TestPostTweet_EmptyText(t *testing.T) {
	svc := application.NewTweetService(&mockTweetRepo{})
	_, err := svc.PostTweet(context.Background(), testUserID, "", nil)
	if err == nil {
		t.Fatal("expected error for empty text")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *apperr.Error, got %T", err)
	}
	if appErr.Status() != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", appErr.Status(), http.StatusBadRequest)
	}
}

func TestPostTweet_ExactLimit(t *testing.T) {
	svc := application.NewTweetService(&mockTweetRepo{})
	_, err := svc.PostTweet(context.Background(), testUserID, strings.Repeat("a", 280), nil)
	if err != nil {
		t.Errorf("unexpected error for 280-char tweet: %v", err)
	}
}

func TestPostTweet_IDEncodesUserAndTime(t *testing.T) {
	repo := &mockTweetRepo{}
	svc := application.NewTweetService(repo)

	_, err := svc.PostTweet(context.Background(), testUserID, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ID must contain the hex of the user UUID (no dashes)
	hexUserID := strings.ReplaceAll(testUserID, "-", "")
	if !strings.HasPrefix(repo.saved.ID, hexUserID) {
		t.Errorf("tweet ID %q does not start with user hex %q", repo.saved.ID, hexUserID)
	}
}

func TestPostTweet_WithMedia(t *testing.T) {
	repo := &mockTweetRepo{}
	svc := application.NewTweetService(repo)
	media := []domain.MediaItem{{Type: "image", URL: "https://example.com/img.jpg"}}

	tweet, err := svc.PostTweet(context.Background(), testUserID, "check this out", media)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tweet.Media) != 1 {
		t.Errorf("expected 1 media item, got %d", len(tweet.Media))
	}
}
