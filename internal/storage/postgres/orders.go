package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const (
	queryInsertOrder = `
INSERT INTO orders (number, user_id)
VALUES ($1, $2)
ON CONFLICT (number) DO NOTHING`

	queryGetOrderOwner = `
SELECT user_id
FROM orders
WHERE number = $1`

	queryListOrdersByUser = `
SELECT number, user_id, status, accrual, uploaded_at
FROM orders
WHERE user_id = $1
ORDER BY uploaded_at DESC`

	queryListPendingOrders = `
SELECT number, user_id, status, accrual, uploaded_at
FROM orders
WHERE status IN ('NEW', 'PROCESSING')
ORDER BY uploaded_at
LIMIT $1`

	queryUpdateOrderStatus = `
UPDATE orders
SET status = $2
WHERE number = $1 AND status IN ('NEW', 'PROCESSING')`

	queryMarkOrderProcessed = `
UPDATE orders
SET status = 'PROCESSED', accrual = $2
WHERE number = $1 AND status IN ('NEW', 'PROCESSING')
RETURNING user_id`

	queryCreditBalance = `
UPDATE users
SET balance = balance + $2
WHERE id = $1`
)

func (s *Storage) CreateOrder(ctx context.Context, number string, userID int64) error {
	tag, err := s.db.Exec(ctx, queryInsertOrder, number, userID)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}

	var ownerID int64
	if err := s.db.QueryRow(ctx, queryGetOrderOwner, number).Scan(&ownerID); err != nil {
		return fmt.Errorf("get order owner: %w", err)
	}
	if ownerID == userID {
		return model.ErrOrderAlreadyUploaded
	}
	return model.ErrOrderUploadedByAnother
}

func (s *Storage) ListOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	rows, err := s.db.Query(ctx, queryListOrdersByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}

	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, fmt.Errorf("scan orders: %w", err)
	}
	return orders, nil
}

func (s *Storage) ListOrdersForProcessing(ctx context.Context, limit int) ([]model.Order, error) {
	rows, err := s.db.Query(ctx, queryListPendingOrders, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending orders: %w", err)
	}

	orders, err := pgx.CollectRows(rows, scanOrder)
	if err != nil {
		return nil, fmt.Errorf("scan pending orders: %w", err)
	}
	return orders, nil
}

func (s *Storage) UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus) error {
	if _, err := s.db.Exec(ctx, queryUpdateOrderStatus, number, string(status)); err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	return nil
}

func (s *Storage) ProcessOrder(ctx context.Context, number string, accrual model.Money) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		var userID int64
		err := tx.QueryRow(ctx, queryMarkOrderProcessed, number, accrual).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mark order processed: %w", err)
		}

		if _, err := tx.Exec(ctx, queryCreditBalance, userID, accrual); err != nil {
			return fmt.Errorf("credit balance: %w", err)
		}
		return nil
	})
}

func scanOrder(row pgx.CollectableRow) (model.Order, error) {
	var (
		order  model.Order
		status string
	)
	if err := row.Scan(&order.Number, &order.UserID, &status, &order.Accrual, &order.UploadedAt); err != nil {
		return model.Order{}, err
	}
	order.Status = model.OrderStatus(status)
	return order, nil
}
