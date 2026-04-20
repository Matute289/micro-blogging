package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FollowRepository struct {
	db *pgxpool.Pool
}

func NewFollowRepository(db *pgxpool.Pool) *FollowRepository {
	return &FollowRepository{db: db}
}

func (r *FollowRepository) Follow(ctx context.Context, followerID, followingID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO follows (follower_id, following_id, created_at) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		followerID, followingID, time.Now().UTC(),
	)
	return err
}

func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followingID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`,
		followerID, followingID,
	)
	return err
}

func (r *FollowRepository) GetFollowing(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT following_id FROM follows WHERE follower_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *FollowRepository) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT follower_id FROM follows WHERE following_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
