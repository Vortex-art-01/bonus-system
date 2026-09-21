package service

import (
	"context"
	"fmt"

	"github.com/Vortex-art-01/bonus-system/internal/luhn"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type BalanceRepository interface {
	GetBalance(ctx context.Context, userID int64) (*model.Balance, error)
	Withdraw(ctx context.Context, userID int64, order string, sum float64) error
	ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

type BalanceService struct {
	repo BalanceRepository
}

func NewBalanceService(repo BalanceRepository) *BalanceService {
	return &BalanceService{repo: repo}
}

func (s *BalanceService) Get(ctx context.Context, userID int64) (*model.Balance, error) {
	balance, err := s.repo.GetBalance(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	return balance, nil
}

func (s *BalanceService) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	if !luhn.Valid(order) {
		return model.ErrInvalidOrderNumber
	}
	if sum <= 0 {
		return model.ErrInvalidWithdrawalSum
	}
	if err := s.repo.Withdraw(ctx, userID, order, sum); err != nil {
		return fmt.Errorf("withdraw for order %s: %w", order, err)
	}
	return nil
}

func (s *BalanceService) ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	withdrawals, err := s.repo.ListWithdrawals(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list withdrawals: %w", err)
	}
	return withdrawals, nil
}
