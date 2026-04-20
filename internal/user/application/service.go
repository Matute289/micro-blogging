package application

import (
	"context"
	"time"

	"UalaTwitter/internal/user/domain"
	"github.com/google/uuid"
)

type UserService struct {
	repo domain.UserRepository
}

func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) CreateUser(ctx context.Context, username string) (*domain.User, error) {
	user := &domain.User{
		ID:         uuid.New().String(),
		Username:   username,
		CreatedAt:  time.Now().UTC(),
		LastSeenAt: time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}
