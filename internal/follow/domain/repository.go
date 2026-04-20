package domain

import "context"

type FollowRepository interface {
	Follow(ctx context.Context, followerID, followingID string) error
	Unfollow(ctx context.Context, followerID, followingID string) error
	GetFollowing(ctx context.Context, userID string) ([]string, error) // users that userID follows
	GetFollowers(ctx context.Context, userID string) ([]string, error) // users that follow userID
}
