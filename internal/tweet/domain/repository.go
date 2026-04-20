package domain

import "context"

type TweetRepository interface {
	Save(ctx context.Context, tweet *Tweet) error
	GetByUserID(ctx context.Context, userID string, limit int, beforeID string) ([]*Tweet, error)
	GetByUserIDs(ctx context.Context, userIDs []string, limit int, beforeID string) ([]*Tweet, error)
	GetByIDs(ctx context.Context, ids []string) ([]*Tweet, error)
}
