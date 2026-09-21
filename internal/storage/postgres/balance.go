package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const (
	queryGetBalance = `
SELECT balance, withdrawn
FROM users
WHERE id = $1`

	queryDebitBalance = `
UPDATE users
SET balance = balance - $2, withdrawn = withdrawn + $2
WHERE id = $1 AND balance >= $2`

	queryInsertWithdrawal = `
INSERT INTO withdrawals (user_id, order_number, sum)
VALUES ($1, $2, $3)`

	queryListWithdrawals = `
SELECT id, user_id, order_number, sum, processed_at
FROM withdrawals
WHERE user_id = $1
ORDER BY processed_at DESC`
)

func (s *Storage) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	var balance model.Balance

	err := s.db.QueryRow(ctx, queryGetBalance, userID).Scan(&balance.Current, &balance.Withdrawn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	return &balance, nil
}

func (s *Storage) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, queryDebitBalance, userID, sum)
		if err != nil {
			return fmt.Errorf("debit balance: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return model.ErrInsufficientFunds
		}

		if _, err := tx.Exec(ctx, queryInsertWithdrawal, userID, order, sum); err != nil {
			return fmt.Errorf("insert withdrawal: %w", err)
		}
		return nil
	})
}

func (s *Storage) ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	rows, err := s.db.Query(ctx, queryListWithdrawals, userID)
	if err != nil {
		return nil, fmt.Errorf("list withdrawals: %w", err)
	}

	withdrawals, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.Withdrawal, error) {
		var w model.Withdrawal
		err := row.Scan(&w.ID, &w.UserID, &w.Order, &w.Sum, &w.ProcessedAt)
		return w, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan withdrawals: %w", err)
	}
	return withdrawals, nil
}
