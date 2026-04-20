package application

import (
	"context"

	"UalaTwitter/internal/follow/domain"
	"UalaTwitter/pkg/apperr"
)

type FollowService struct {
	repo domain.FollowRepository
}

func NewFollowService(repo domain.FollowRepository) *FollowService {
	return &FollowService{repo: repo}
}

func (s *FollowService) Follow(ctx context.Context, followerID, followingID string) error {
	if followerID == followingID {
		return apperr.NotAllowed("cannot follow yourself")
	}
	return s.repo.Follow(ctx, followerID, followingID)
}

func (s *FollowService) Unfollow(ctx context.Context, followerID, followingID string) error {
	return s.repo.Unfollow(ctx, followerID, followingID)
}

func (s *FollowService) GetFollowing(ctx context.Context, userID string) ([]string, error) {
	return s.repo.GetFollowing(ctx, userID)
}

func (s *FollowService) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	return s.repo.GetFollowers(ctx, userID)
}
