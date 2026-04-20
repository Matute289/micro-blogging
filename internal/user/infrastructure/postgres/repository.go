package postgres

import (
	"context"
	"errors"

	"UalaTwitter/internal/user/domain"
	"UalaTwitter/pkg/apperr"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO users (id, username, created_at, last_seen_at) VALUES ($1, $2, $3, $4)`,
		user.ID, user.Username, user.CreatedAt, user.LastSeenAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperr.Conflict("username already taken")
		}
		return err
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	u := &domain.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, username, created_at, last_seen_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt, &u.LastSeenAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}
