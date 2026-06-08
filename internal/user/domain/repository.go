package domain

import "context"

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	FindByOAuth(ctx context.Context, provider, sub string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
}
