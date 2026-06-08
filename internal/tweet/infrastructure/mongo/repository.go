package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"UalaTwitter/internal/tweet/domain"
	"UalaTwitter/pkg/tweetid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type tweetDoc struct {
	ID          string             `bson:"_id"`
	UserID      string             `bson:"user_id"`
	Text        string             `bson:"text"`
	Media       []domain.MediaItem `bson:"media"`
	CreatedAtMs int64              `bson:"created_at_ms"`
}

type TweetRepository struct {
	col *mongo.Collection
}

func NewTweetRepository(db *mongo.Database) *TweetRepository {
	col := db.Collection("tweets")
	col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at_ms", Value: -1}},
	})
	return &TweetRepository{col: col}
}

func (r *TweetRepository) Save(ctx context.Context, t *domain.Tweet) error {
	_, err := r.col.InsertOne(ctx, tweetDoc{
		ID:          t.ID,
		UserID:      t.UserID,
		Text:        t.Text,
		Media:       t.Media,
		CreatedAtMs: t.CreatedAt.UnixMilli(),
	})
	return err
}

func (r *TweetRepository) GetByID(ctx context.Context, id string) (*domain.Tweet, error) {
	var doc tweetDoc
	err := r.col.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("tweet not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	return &domain.Tweet{
		ID:        doc.ID,
		UserID:    doc.UserID,
		Text:      doc.Text,
		Media:     doc.Media,
		CreatedAt: time.UnixMilli(doc.CreatedAtMs).UTC(),
	}, nil
}

func (r *TweetRepository) GetByUserID(ctx context.Context, userID string, limit int, beforeID string) ([]*domain.Tweet, error) {
	filter := bson.M{"user_id": userID}
	applyBeforeCursor(filter, beforeID)
	return r.find(ctx, filter, limit)
}

func (r *TweetRepository) GetByUserIDs(ctx context.Context, userIDs []string, limit int, beforeID string) ([]*domain.Tweet, error) {
	filter := bson.M{"user_id": bson.M{"$in": userIDs}}
	applyBeforeCursor(filter, beforeID)
	return r.find(ctx, filter, limit)
}

func (r *TweetRepository) GetByIDs(ctx context.Context, ids []string) ([]*domain.Tweet, error) {
	return r.find(ctx, bson.M{"_id": bson.M{"$in": ids}}, len(ids))
}

func (r *TweetRepository) find(ctx context.Context, filter bson.M, limit int) ([]*domain.Tweet, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at_ms", Value: -1}}).
		SetLimit(int64(limit))
	cur, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var tweets []*domain.Tweet
	for cur.Next(ctx) {
		var doc tweetDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		tweets = append(tweets, &domain.Tweet{
			ID:        doc.ID,
			UserID:    doc.UserID,
			Text:      doc.Text,
			Media:     doc.Media,
			CreatedAt: time.UnixMilli(doc.CreatedAtMs).UTC(),
		})
	}
	return tweets, cur.Err()
}

func applyBeforeCursor(filter bson.M, beforeID string) {
	if beforeID == "" {
		return
	}
	_, t, err := tweetid.Decompose(beforeID)
	if err != nil {
		return
	}
	filter["created_at_ms"] = bson.M{"$lt": t.UnixMilli()}
}
