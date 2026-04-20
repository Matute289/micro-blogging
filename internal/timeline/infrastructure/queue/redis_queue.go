package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey    = "fanout:queue"
	maxAttempts = 15
)

type fanOutMessage struct {
	PosterID  string    `json:"poster_id"`
	TweetID   string    `json:"tweet_id"`
	CreatedAt time.Time `json:"created_at"`
	Attempt   int       `json:"attempt"`
}

type RedisQueue struct {
	client *redis.Client
}

func NewRedisQueue(client *redis.Client) *RedisQueue {
	return &RedisQueue{client: client}
}

func (q *RedisQueue) Enqueue(ctx context.Context, posterID, tweetID string, createdAt time.Time) error {
	data, err := json.Marshal(fanOutMessage{PosterID: posterID, TweetID: tweetID, CreatedAt: createdAt})
	if err != nil {
		return err
	}
	return q.client.RPush(ctx, queueKey, data).Err()
}

func (q *RedisQueue) dequeue(ctx context.Context) (*fanOutMessage, error) {
	res, err := q.client.BLPop(ctx, time.Second, queueKey).Result()
	if err != nil {
		return nil, err
	}
	var msg fanOutMessage
	if err := json.Unmarshal([]byte(res[1]), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (q *RedisQueue) requeue(ctx context.Context, msg fanOutMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("fanout requeue marshal failed", "tweet_id", msg.TweetID, "err", err)
		return
	}
	if err := q.client.RPush(ctx, queueKey, data).Err(); err != nil {
		slog.Error("fanout requeue failed", "tweet_id", msg.TweetID, "err", err)
	}
}

// RunWorker dequeues fan-out messages and calls fanOut for each one.
// On failure it requeues the message up to maxAttempts times, then drops it with an error log.
// Blocks until ctx is cancelled.
func (q *RedisQueue) RunWorker(ctx context.Context, fanOut func(context.Context, string, string, time.Time) error) {
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := q.dequeue(ctx)
		if err != nil {
			if !errors.Is(err, redis.Nil) && ctx.Err() == nil {
				slog.Warn("fanout dequeue error", "err", err)
			}
			continue
		}
		if err := fanOut(ctx, msg.PosterID, msg.TweetID, msg.CreatedAt); err != nil {
			msg.Attempt++
			if msg.Attempt < maxAttempts {
				slog.Warn("fanout failed, requeueing", "tweet_id", msg.TweetID, "attempt", msg.Attempt, "err", err)
				q.requeue(ctx, *msg)
			} else {
				slog.Error("fanout dropped after max retries", "tweet_id", msg.TweetID, "poster_id", msg.PosterID)
			}
		}
	}
}
