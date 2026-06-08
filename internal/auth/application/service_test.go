package application_test

import (
	"context"
	"testing"
	"time"

	"UalaTwitter/internal/auth/application"
	"UalaTwitter/internal/user/domain"
	"UalaTwitter/pkg/apperr"
)

type mockUserRepo struct {
	users map[string]*domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, u *domain.User) error {
	if m.users == nil {
		m.users = make(map[string]*domain.User)
	}
	for _, existing := range m.users {
		if existing.Username == u.Username {
			return apperr.Conflict("username already taken")
		}
	}
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, apperr.ErrNotFound
}

func (m *mockUserRepo) FindByOAuth(_ context.Context, provider, sub string) (*domain.User, error) {
	for _, u := range m.users {
		if u.OAuthProvider == provider && u.OAuthSub == sub {
			return u, nil
		}
	}
	return nil, apperr.ErrNotFound
}

func (m *mockUserRepo) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, apperr.ErrNotFound
}

const testSecret = "test-secret-at-least-32-chars-long"

func TestFindOrCreateUser_CreatesNewUser(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewAuthService(repo, testSecret, "", "", "", "", "")

	user, err := svc.FindOrCreateUser(context.Background(), "google", "google-sub-123", "alice_test", "alice@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.OAuthProvider != "google" || user.OAuthSub != "google-sub-123" {
		t.Errorf("wrong oauth identity: %+v", user)
	}
	if user.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestFindOrCreateUser_ReturnsExistingUser(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewAuthService(repo, testSecret, "", "", "", "", "")

	first, _ := svc.FindOrCreateUser(context.Background(), "github", "gh-sub-1", "bob", "")
	second, err := svc.FindOrCreateUser(context.Background(), "github", "gh-sub-1", "bob", "")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if first.ID != second.ID {
		t.Error("expected same user to be returned on second find")
	}
	if len(repo.users) != 1 {
		t.Errorf("expected 1 user in repo, got %d", len(repo.users))
	}
}

func TestFindOrCreateUser_DeduplicatesUsername(t *testing.T) {
	repo := &mockUserRepo{}
	repo.users = map[string]*domain.User{
		"existing-id": {
			ID: "existing-id", Username: "carol",
			CreatedAt: time.Now(), LastSeenAt: time.Now(),
		},
	}
	svc := application.NewAuthService(repo, testSecret, "", "", "", "", "")

	user, err := svc.FindOrCreateUser(context.Background(), "apple", "apple-sub-99", "carol", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username == "carol" {
		t.Error("expected deduplicated username, got 'carol'")
	}
}

func TestIssueToken_ContainsUserID(t *testing.T) {
	svc := application.NewAuthService(&mockUserRepo{}, testSecret, "", "", "", "", "")
	user := &domain.User{ID: "test-id", Username: "dave", CreatedAt: time.Now(), LastSeenAt: time.Now()}

	token, err := svc.IssueToken(user)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}
