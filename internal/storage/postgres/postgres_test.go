package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

var errBoom = errors.New("boom")

func newMock(t *testing.T) (*Storage, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		mock.Close()
	})
	return New(mock), mock
}

func ptr[T any](v T) *T { return &v }

func TestCreateUser(t *testing.T) {
	now := time.Now()

	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryCreateUser).WithArgs("alice", "hash").
			WillReturnRows(mock.NewRows([]string{"id", "created_at"}).AddRow(int64(7), now))

		user, err := s.CreateUser(context.Background(), "alice", "hash")
		require.NoError(t, err)
		assert.Equal(t, &model.User{ID: 7, Login: "alice", PasswordHash: "hash", CreatedAt: now}, user)
	})

	t.Run("login taken", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryCreateUser).WithArgs("alice", "hash").
			WillReturnError(&pgconn.PgError{Code: uniqueViolationCode})

		_, err := s.CreateUser(context.Background(), "alice", "hash")
		assert.ErrorIs(t, err, model.ErrLoginTaken)
	})

	t.Run("db error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryCreateUser).WithArgs("alice", "hash").WillReturnError(errBoom)

		_, err := s.CreateUser(context.Background(), "alice", "hash")
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestGetUserByLogin(t *testing.T) {
	now := time.Now()

	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetUserByLogin).WithArgs("alice").
			WillReturnRows(mock.NewRows([]string{"id", "login", "password_hash", "created_at"}).
				AddRow(int64(7), "alice", "hash", now))

		user, err := s.GetUserByLogin(context.Background(), "alice")
		require.NoError(t, err)
		assert.Equal(t, &model.User{ID: 7, Login: "alice", PasswordHash: "hash", CreatedAt: now}, user)
	})

	t.Run("not found", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetUserByLogin).WithArgs("bob").WillReturnError(pgx.ErrNoRows)

		_, err := s.GetUserByLogin(context.Background(), "bob")
		assert.ErrorIs(t, err, model.ErrUserNotFound)
	})

	t.Run("db error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetUserByLogin).WithArgs("bob").WillReturnError(errBoom)

		_, err := s.GetUserByLogin(context.Background(), "bob")
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestCreateOrder(t *testing.T) {
	t.Run("created", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryInsertOrder).WithArgs("123", int64(1)).WillReturnResult(pgxmock.NewResult("INSERT", 1))

		assert.NoError(t, s.CreateOrder(context.Background(), "123", 1))
	})

	t.Run("already uploaded by same user", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryInsertOrder).WithArgs("123", int64(1)).WillReturnResult(pgxmock.NewResult("INSERT", 0))
		mock.ExpectQuery(queryGetOrderOwner).WithArgs("123").
			WillReturnRows(mock.NewRows([]string{"user_id"}).AddRow(int64(1)))

		assert.ErrorIs(t, s.CreateOrder(context.Background(), "123", 1), model.ErrOrderAlreadyUploaded)
	})

	t.Run("uploaded by another user", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryInsertOrder).WithArgs("123", int64(1)).WillReturnResult(pgxmock.NewResult("INSERT", 0))
		mock.ExpectQuery(queryGetOrderOwner).WithArgs("123").
			WillReturnRows(mock.NewRows([]string{"user_id"}).AddRow(int64(2)))

		assert.ErrorIs(t, s.CreateOrder(context.Background(), "123", 1), model.ErrOrderUploadedByAnother)
	})

	t.Run("insert error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryInsertOrder).WithArgs("123", int64(1)).WillReturnError(errBoom)

		assert.ErrorIs(t, s.CreateOrder(context.Background(), "123", 1), errBoom)
	})

	t.Run("owner lookup error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryInsertOrder).WithArgs("123", int64(1)).WillReturnResult(pgxmock.NewResult("INSERT", 0))
		mock.ExpectQuery(queryGetOrderOwner).WithArgs("123").WillReturnError(errBoom)

		assert.ErrorIs(t, s.CreateOrder(context.Background(), "123", 1), errBoom)
	})
}

func orderColumns() []string {
	return []string{"number", "user_id", "status", "accrual", "uploaded_at"}
}

func TestListOrders(t *testing.T) {
	now := time.Now()

	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListOrdersByUser).WithArgs(int64(1)).
			WillReturnRows(mock.NewRows(orderColumns()).
				AddRow("2", int64(1), "PROCESSED", ptr(500.5), now).
				AddRow("1", int64(1), "NEW", (*float64)(nil), now.Add(-time.Hour)))

		orders, err := s.ListOrders(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, []model.Order{
			{Number: "2", UserID: 1, Status: model.OrderStatusProcessed, Accrual: ptr(500.5), UploadedAt: now},
			{Number: "1", UserID: 1, Status: model.OrderStatusNew, UploadedAt: now.Add(-time.Hour)},
		}, orders)
	})

	t.Run("empty", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListOrdersByUser).WithArgs(int64(1)).WillReturnRows(mock.NewRows(orderColumns()))

		orders, err := s.ListOrders(context.Background(), 1)
		require.NoError(t, err)
		assert.Empty(t, orders)
	})

	t.Run("query error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListOrdersByUser).WithArgs(int64(1)).WillReturnError(errBoom)

		_, err := s.ListOrders(context.Background(), 1)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("scan error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListOrdersByUser).WithArgs(int64(1)).
			WillReturnRows(mock.NewRows(orderColumns()).AddRow("1", int64(1), "NEW", (*float64)(nil), now).RowError(0, errBoom))

		_, err := s.ListOrders(context.Background(), 1)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestListOrdersForProcessing(t *testing.T) {
	now := time.Now()

	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListPendingOrders).WithArgs(10).
			WillReturnRows(mock.NewRows(orderColumns()).AddRow("1", int64(1), "PROCESSING", (*float64)(nil), now))

		orders, err := s.ListOrdersForProcessing(context.Background(), 10)
		require.NoError(t, err)
		assert.Equal(t, []model.Order{{Number: "1", UserID: 1, Status: model.OrderStatusProcessing, UploadedAt: now}}, orders)
	})

	t.Run("query error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListPendingOrders).WithArgs(10).WillReturnError(errBoom)

		_, err := s.ListOrdersForProcessing(context.Background(), 10)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("scan error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListPendingOrders).WithArgs(10).
			WillReturnRows(mock.NewRows(orderColumns()).AddRow("1", int64(1), "NEW", (*float64)(nil), now).RowError(0, errBoom))

		_, err := s.ListOrdersForProcessing(context.Background(), 10)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestUpdateOrderStatus(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryUpdateOrderStatus).WithArgs("1", "INVALID").WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		assert.NoError(t, s.UpdateOrderStatus(context.Background(), "1", model.OrderStatusInvalid))
	})

	t.Run("error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectExec(queryUpdateOrderStatus).WithArgs("1", "INVALID").WillReturnError(errBoom)

		assert.ErrorIs(t, s.UpdateOrderStatus(context.Background(), "1", model.OrderStatusInvalid), errBoom)
	})
}

func TestProcessOrder(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(queryMarkOrderProcessed).WithArgs("1", 500.5).
			WillReturnRows(mock.NewRows([]string{"user_id"}).AddRow(int64(3)))
		mock.ExpectExec(queryCreditBalance).WithArgs(int64(3), 500.5).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit()

		assert.NoError(t, s.ProcessOrder(context.Background(), "1", 500.5))
	})

	t.Run("already final", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(queryMarkOrderProcessed).WithArgs("1", 500.5).WillReturnError(pgx.ErrNoRows)
		mock.ExpectCommit()

		assert.NoError(t, s.ProcessOrder(context.Background(), "1", 500.5))
	})

	t.Run("begin error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin().WillReturnError(errBoom)

		assert.ErrorIs(t, s.ProcessOrder(context.Background(), "1", 500.5), errBoom)
	})

	t.Run("mark error rolls back", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(queryMarkOrderProcessed).WithArgs("1", 500.5).WillReturnError(errBoom)
		mock.ExpectRollback()

		assert.ErrorIs(t, s.ProcessOrder(context.Background(), "1", 500.5), errBoom)
	})

	t.Run("credit error rolls back", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(queryMarkOrderProcessed).WithArgs("1", 500.5).
			WillReturnRows(mock.NewRows([]string{"user_id"}).AddRow(int64(3)))
		mock.ExpectExec(queryCreditBalance).WithArgs(int64(3), 500.5).WillReturnError(errBoom)
		mock.ExpectRollback()

		assert.ErrorIs(t, s.ProcessOrder(context.Background(), "1", 500.5), errBoom)
	})

	t.Run("commit error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery(queryMarkOrderProcessed).WithArgs("1", 500.5).
			WillReturnRows(mock.NewRows([]string{"user_id"}).AddRow(int64(3)))
		mock.ExpectExec(queryCreditBalance).WithArgs(int64(3), 500.5).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectCommit().WillReturnError(errBoom)
		mock.ExpectRollback()

		assert.ErrorIs(t, s.ProcessOrder(context.Background(), "1", 500.5), errBoom)
	})
}

func TestGetBalance(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetBalance).WithArgs(int64(1)).
			WillReturnRows(mock.NewRows([]string{"balance", "withdrawn"}).AddRow(500.5, 42.0))

		balance, err := s.GetBalance(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, &model.Balance{Current: 500.5, Withdrawn: 42}, balance)
	})

	t.Run("not found", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetBalance).WithArgs(int64(1)).WillReturnError(pgx.ErrNoRows)

		_, err := s.GetBalance(context.Background(), 1)
		assert.ErrorIs(t, err, model.ErrUserNotFound)
	})

	t.Run("db error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryGetBalance).WithArgs(int64(1)).WillReturnError(errBoom)

		_, err := s.GetBalance(context.Background(), 1)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestWithdraw(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(queryDebitBalance).WithArgs(int64(1), 100.0).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(queryInsertWithdrawal).WithArgs(int64(1), "2377225624", 100.0).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		assert.NoError(t, s.Withdraw(context.Background(), 1, "2377225624", 100))
	})

	t.Run("insufficient funds", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(queryDebitBalance).WithArgs(int64(1), 100.0).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectRollback()

		assert.ErrorIs(t, s.Withdraw(context.Background(), 1, "2377225624", 100), model.ErrInsufficientFunds)
	})

	t.Run("debit error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(queryDebitBalance).WithArgs(int64(1), 100.0).WillReturnError(errBoom)
		mock.ExpectRollback()

		assert.ErrorIs(t, s.Withdraw(context.Background(), 1, "2377225624", 100), errBoom)
	})

	t.Run("insert error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectBegin()
		mock.ExpectExec(queryDebitBalance).WithArgs(int64(1), 100.0).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(queryInsertWithdrawal).WithArgs(int64(1), "2377225624", 100.0).WillReturnError(errBoom)
		mock.ExpectRollback()

		assert.ErrorIs(t, s.Withdraw(context.Background(), 1, "2377225624", 100), errBoom)
	})
}

func TestListWithdrawals(t *testing.T) {
	now := time.Now()
	columns := []string{"id", "user_id", "order_number", "sum", "processed_at"}

	t.Run("ok", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListWithdrawals).WithArgs(int64(1)).
			WillReturnRows(mock.NewRows(columns).AddRow(int64(5), int64(1), "2377225624", 500.0, now))

		withdrawals, err := s.ListWithdrawals(context.Background(), 1)
		require.NoError(t, err)
		assert.Equal(t, []model.Withdrawal{{ID: 5, UserID: 1, Order: "2377225624", Sum: 500, ProcessedAt: now}}, withdrawals)
	})

	t.Run("query error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListWithdrawals).WithArgs(int64(1)).WillReturnError(errBoom)

		_, err := s.ListWithdrawals(context.Background(), 1)
		assert.ErrorIs(t, err, errBoom)
	})

	t.Run("scan error", func(t *testing.T) {
		s, mock := newMock(t)
		mock.ExpectQuery(queryListWithdrawals).WithArgs(int64(1)).
			WillReturnRows(mock.NewRows(columns).AddRow(int64(5), int64(1), "2377225624", 500.0, now).RowError(0, errBoom))

		_, err := s.ListWithdrawals(context.Background(), 1)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestWithTx_PanicRollsBack(t *testing.T) {
	s, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectRollback()

	assert.Panics(t, func() {
		_ = s.withTx(context.Background(), func(pgx.Tx) error { panic("boom") })
	})
}

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, isUniqueViolation(&pgconn.PgError{Code: uniqueViolationCode}))
	assert.False(t, isUniqueViolation(&pgconn.PgError{Code: "23503"}))
	assert.False(t, isUniqueViolation(errBoom))
	assert.False(t, isUniqueViolation(nil))
}
