//go:build integration

package mongo_test

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	tweetdomain "UalaTwitter/internal/tweet/domain"
	tweetmongo "UalaTwitter/internal/tweet/infrastructure/mongo"
	"UalaTwitter/pkg/tweetid"
)

func TestGetByID_ReturnsExistingTweet(t *testing.T) {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	defer client.Disconnect(ctx)
	repo := tweetmongo.NewTweetRepository(client.Database("twitter_test_getbyid"))

	now := time.Now().UTC().Truncate(time.Millisecond)
	userID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	tweet := &tweetdomain.Tweet{
		ID:        tweetid.Compose(userID, now),
		UserID:    userID,
		Text:      "hello world",
		CreatedAt: now,
	}
	if err := repo.Save(ctx, tweet); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := repo.GetByID(ctx, tweet.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != tweet.ID {
		t.Errorf("want id %s, got %s", tweet.ID, got.ID)
	}
	if got.Text != tweet.Text {
		t.Errorf("want text %q, got %q", tweet.Text, got.Text)
	}
}

func TestGetByID_NotFoundReturnsError(t *testing.T) {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	defer client.Disconnect(ctx)
	repo := tweetmongo.NewTweetRepository(client.Database("twitter_test_getbyid"))

	_, err = repo.GetByID(ctx, "nonexistent_id")
	if err == nil {
		t.Fatal("expected error for nonexistent tweet, got nil")
	}
}
