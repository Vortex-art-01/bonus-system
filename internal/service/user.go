package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Vortex-art-01/bonus-system/internal/auth"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type UserRepository interface {
	CreateUser(ctx context.Context, login, passwordHash string) (*model.User, error)
	GetUserByLogin(ctx context.Context, login string) (*model.User, error)
}

type TokenIssuer interface {
	Issue(userID int64) (string, error)
}

type UserService struct {
	repo   UserRepository
	tokens TokenIssuer
}

func NewUserService(repo UserRepository, tokens TokenIssuer) *UserService {
	return &UserService{repo: repo, tokens: tokens}
}

func (s *UserService) Register(ctx context.Context, login, password string) (string, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return "", err
	}

	user, err := s.repo.CreateUser(ctx, login, hash)
	if err != nil {
		return "", fmt.Errorf("register user: %w", err)
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}

func (s *UserService) Login(ctx context.Context, login, password string) (string, error) {
	user, err := s.repo.GetUserByLogin(ctx, login)
	if errors.Is(err, model.ErrUserNotFound) {
		return "", model.ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("find user: %w", err)
	}

	if !auth.CheckPassword(user.PasswordHash, password) {
		return "", model.ErrInvalidCredentials
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}
