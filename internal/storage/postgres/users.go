package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const (
	queryCreateUser = `
INSERT INTO users (login, password_hash)
VALUES ($1, $2)
RETURNING id, created_at`

	queryGetUserByLogin = `
SELECT id, login, password_hash, created_at
FROM users
WHERE login = $1`
)

func (s *Storage) CreateUser(ctx context.Context, login, passwordHash string) (*model.User, error) {
	user := &model.User{Login: login, PasswordHash: passwordHash}

	err := s.db.QueryRow(ctx, queryCreateUser, login, passwordHash).Scan(&user.ID, &user.CreatedAt)
	if isUniqueViolation(err) {
		return nil, model.ErrLoginTaken
	}
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (s *Storage) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	var user model.User

	err := s.db.QueryRow(ctx, queryGetUserByLogin, login).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by login: %w", err)
	}
	return &user, nil
}
