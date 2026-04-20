package application_test

import (
	"context"
	"testing"

	"UalaTwitter/internal/follow/application"
)

type mockFollowRepo struct {
	follows   [][2]string
	unfollows [][2]string
}

func (m *mockFollowRepo) Follow(ctx context.Context, followerID, followingID string) error {
	m.follows = append(m.follows, [2]string{followerID, followingID})
	return nil
}
func (m *mockFollowRepo) Unfollow(ctx context.Context, followerID, followingID string) error {
	m.unfollows = append(m.unfollows, [2]string{followerID, followingID})
	return nil
}
func (m *mockFollowRepo) GetFollowing(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (m *mockFollowRepo) GetFollowers(_ context.Context, _ string) ([]string, error) { return nil, nil }

func TestFollow_SelfFollow(t *testing.T) {
	svc := application.NewFollowService(&mockFollowRepo{})
	err := svc.Follow(context.Background(), "user-1", "user-1")
	if err == nil {
		t.Error("expected error when following yourself")
	}
}

func TestFollow_Valid(t *testing.T) {
	repo := &mockFollowRepo{}
	svc := application.NewFollowService(repo)

	if err := svc.Follow(context.Background(), "user-1", "user-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.follows) != 1 {
		t.Fatalf("expected 1 follow recorded, got %d", len(repo.follows))
	}
	if repo.follows[0] != [2]string{"user-1", "user-2"} {
		t.Errorf("follow pair: got %v, want [user-1 user-2]", repo.follows[0])
	}
}

func TestFollow_DoesNotCallRepoOnSelfFollow(t *testing.T) {
	repo := &mockFollowRepo{}
	svc := application.NewFollowService(repo)

	_ = svc.Follow(context.Background(), "user-1", "user-1")
	if len(repo.follows) != 0 {
		t.Error("repo should not be called on self-follow")
	}
}

func TestUnfollow(t *testing.T) {
	repo := &mockFollowRepo{}
	svc := application.NewFollowService(repo)

	if err := svc.Unfollow(context.Background(), "user-1", "user-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.unfollows) != 1 {
		t.Errorf("expected 1 unfollow recorded, got %d", len(repo.unfollows))
	}
}
