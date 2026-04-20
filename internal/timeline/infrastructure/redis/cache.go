package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const maxTimelineSize = 200

type TimelineCache struct {
	client *redis.Client
}

func NewTimelineCache(client *redis.Client) *TimelineCache {
	return &TimelineCache{client: client}
}

func key(userID string) string {
	return "timeline:" + userID
}

// Push adds a tweet ID to the user's timeline sorted set (score = unix ms).
// Trims to maxTimelineSize entries to bound memory usage.
func (c *TimelineCache) Push(ctx context.Context, userID, tweetID string, score float64) error {
	k := key(userID)
	pipe := c.client.TxPipeline()
	pipe.ZAdd(ctx, k, redis.Z{Score: score, Member: tweetID})
	pipe.ZRemRangeByRank(ctx, k, 0, int64(-maxTimelineSize-1))
	_, err := pipe.Exec(ctx)
	return err
}

// Range returns up to limit tweet IDs with score < max, newest first.
// max uses Redis score syntax: "+inf", "1234567890" or "(1234567890" (exclusive).
func (c *TimelineCache) Range(ctx context.Context, userID string, limit int, max string) ([]string, error) {
	return c.client.ZRevRangeByScore(ctx, key(userID), &redis.ZRangeBy{
		Min:   "-inf",
		Max:   max,
		Count: int64(limit),
	}).Result()
}

// Exists reports whether the user has a populated timeline cache.
func (c *TimelineCache) Exists(ctx context.Context, userID string) (bool, error) {
	n, err := c.client.Exists(ctx, key(userID)).Result()
	return n > 0, err
}
