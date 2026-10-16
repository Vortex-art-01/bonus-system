package service

import (
	"context"
	"fmt"

	"github.com/Vortex-art-01/bonus-system/internal/luhn"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type OrderRepository interface {
	CreateOrder(ctx context.Context, number string, userID int64) error
	ListOrders(ctx context.Context, userID int64) ([]model.Order, error)
}

type OrderService struct {
	repo OrderRepository
}

func NewOrderService(repo OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) Upload(ctx context.Context, userID int64, number string) error {
	if !luhn.Valid(number) {
		return model.ErrInvalidOrderNumber
	}
	if err := s.repo.CreateOrder(ctx, number, userID); err != nil {
		return fmt.Errorf("upload order %s: %w", number, err)
	}
	return nil
}

func (s *OrderService) List(ctx context.Context, userID int64) ([]model.Order, error) {
	orders, err := s.repo.ListOrders(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return orders, nil
}
