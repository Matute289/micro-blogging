package application_test

import (
	"context"
	"errors"
	"testing"

	"UalaTwitter/internal/user/application"
	"UalaTwitter/internal/user/domain"
	"UalaTwitter/pkg/apperr"
)

type mockUserRepo struct {
	created []*domain.User
	users   map[string]*domain.User
	err     error
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, user)
	if m.users == nil {
		m.users = make(map[string]*domain.User)
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	u, ok := m.users[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

func (m *mockUserRepo) FindByOAuth(_ context.Context, _, _ string) (*domain.User, error) {
	return nil, apperr.ErrNotFound
}

func (m *mockUserRepo) FindByUsername(_ context.Context, _ string) (*domain.User, error) {
	return nil, apperr.ErrNotFound
}

func TestCreateUser_ReturnsUserWithUsername(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewUserService(repo)

	u, err := svc.CreateUser(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Username != "alice" {
		t.Errorf("username: got %q, want %q", u.Username, "alice")
	}
	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestCreateUser_StoresInRepo(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewUserService(repo)

	u, _ := svc.CreateUser(context.Background(), "bob")
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 stored user, got %d", len(repo.created))
	}
	if repo.created[0].ID != u.ID {
		t.Errorf("stored ID %q != returned ID %q", repo.created[0].ID, u.ID)
	}
}

func TestCreateUser_RepoError(t *testing.T) {
	repo := &mockUserRepo{err: errors.New("db down")}
	svc := application.NewUserService(repo)

	_, err := svc.CreateUser(context.Background(), "charlie")
	if err == nil {
		t.Error("expected error when repo fails")
	}
}

func TestGetUser_Found(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewUserService(repo)

	created, _ := svc.CreateUser(context.Background(), "dave")
	got, err := svc.GetUser(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: got %q, want %q", got.ID, created.ID)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	repo := &mockUserRepo{}
	svc := application.NewUserService(repo)

	_, err := svc.GetUser(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing user")
	}
}
