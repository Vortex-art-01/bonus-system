package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/model"
	"github.com/Vortex-art-01/bonus-system/internal/storage/postgres"
	"github.com/Vortex-art-01/bonus-system/internal/testutil"
)

func setup(t *testing.T) *postgres.Storage {
	t.Helper()
	return postgres.New(testutil.NewPool(t, "gophermart_test_storage"))
}

func pause() { time.Sleep(5 * time.Millisecond) }

func TestIntegration_Migrate_IsIdempotent(t *testing.T) {
	pool := testutil.NewPool(t, "gophermart_test_migrate")
	require.NoError(t, postgres.Migrate(context.Background(), pool))
}

func TestIntegration_Users(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	alice, err := s.CreateUser(ctx, "alice", "hash")
	require.NoError(t, err)
	assert.Positive(t, alice.ID)
	assert.False(t, alice.CreatedAt.IsZero())

	_, err = s.CreateUser(ctx, "alice", "other")
	assert.ErrorIs(t, err, model.ErrLoginTaken)

	got, err := s.GetUserByLogin(ctx, "alice")
	require.NoError(t, err)
	assert.Equal(t, alice.ID, got.ID)
	assert.Equal(t, "hash", got.PasswordHash)
	assert.WithinDuration(t, alice.CreatedAt, got.CreatedAt, time.Millisecond)

	_, err = s.GetUserByLogin(ctx, "bob")
	assert.ErrorIs(t, err, model.ErrUserNotFound)

	balance, err := s.GetBalance(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, &model.Balance{}, balance)

	_, err = s.GetBalance(ctx, alice.ID+1000)
	assert.ErrorIs(t, err, model.ErrUserNotFound)
}

func TestIntegration_Orders(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	alice, err := s.CreateUser(ctx, "alice", "hash")
	require.NoError(t, err)
	bob, err := s.CreateUser(ctx, "bob", "hash")
	require.NoError(t, err)

	require.NoError(t, s.CreateOrder(ctx, "12345678903", alice.ID))
	assert.ErrorIs(t, s.CreateOrder(ctx, "12345678903", alice.ID), model.ErrOrderAlreadyUploaded)
	assert.ErrorIs(t, s.CreateOrder(ctx, "12345678903", bob.ID), model.ErrOrderUploadedByAnother)
	pause()
	require.NoError(t, s.CreateOrder(ctx, "9278923470", alice.ID))

	orders, err := s.ListOrders(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, orders, 2)
	assert.Equal(t, "9278923470", orders[0].Number, "newest first")
	assert.Equal(t, "12345678903", orders[1].Number)
	for _, o := range orders {
		assert.Equal(t, alice.ID, o.UserID)
		assert.Equal(t, model.OrderStatusNew, o.Status)
		assert.Nil(t, o.Accrual)
		assert.False(t, o.UploadedAt.IsZero())
	}

	bobOrders, err := s.ListOrders(ctx, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, bobOrders)

	pending, err := s.ListOrdersForProcessing(ctx, 10)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, "12345678903", pending[0].Number, "oldest first")

	pending, err = s.ListOrdersForProcessing(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, pending, 1)

	require.NoError(t, s.UpdateOrderStatus(ctx, "12345678903", model.OrderStatusProcessing))
	orders, err = s.ListOrders(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusProcessing, orders[1].Status)

	require.NoError(t, s.ProcessOrder(ctx, "12345678903", 72998))
	orders, err = s.ListOrders(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusProcessed, orders[1].Status)
	require.NotNil(t, orders[1].Accrual)
	assert.Equal(t, model.Money(72998), *orders[1].Accrual)

	balance, err := s.GetBalance(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.Money(72998), balance.Current)

	require.NoError(t, s.ProcessOrder(ctx, "12345678903", 100000))
	require.NoError(t, s.UpdateOrderStatus(ctx, "12345678903", model.OrderStatusInvalid))
	balance, err = s.GetBalance(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.Money(72998), balance.Current)
	orders, err = s.ListOrders(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.OrderStatusProcessed, orders[1].Status)

	require.NoError(t, s.UpdateOrderStatus(ctx, "9278923470", model.OrderStatusInvalid))
	pending, err = s.ListOrdersForProcessing(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, pending)

	require.NoError(t, s.UpdateOrderStatus(ctx, "0", model.OrderStatusInvalid))
	require.NoError(t, s.ProcessOrder(ctx, "0", 100))
}

func TestIntegration_Withdrawals(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	alice, err := s.CreateUser(ctx, "alice", "hash")
	require.NoError(t, err)

	assert.ErrorIs(t, s.Withdraw(ctx, alice.ID, "2377225624", 1000), model.ErrInsufficientFunds)
	assert.ErrorIs(t, s.Withdraw(ctx, alice.ID+1000, "2377225624", 1000), model.ErrInsufficientFunds)

	require.NoError(t, s.CreateOrder(ctx, "12345678903", alice.ID))
	require.NoError(t, s.ProcessOrder(ctx, "12345678903", 50050))

	require.NoError(t, s.Withdraw(ctx, alice.ID, "2377225624", 10025))
	pause()
	require.NoError(t, s.Withdraw(ctx, alice.ID, "79927398713", 40025))
	assert.ErrorIs(t, s.Withdraw(ctx, alice.ID, "346436439", 1), model.ErrInsufficientFunds)

	balance, err := s.GetBalance(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, model.Money(0), balance.Current)
	assert.Equal(t, model.Money(50050), balance.Withdrawn)

	withdrawals, err := s.ListWithdrawals(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, withdrawals, 2)
	assert.Equal(t, "79927398713", withdrawals[0].Order, "newest first")
	assert.Equal(t, model.Money(40025), withdrawals[0].Sum)
	assert.Equal(t, "2377225624", withdrawals[1].Order)
	assert.Equal(t, model.Money(10025), withdrawals[1].Sum)
	for _, w := range withdrawals {
		assert.Positive(t, w.ID)
		assert.Equal(t, alice.ID, w.UserID)
		assert.False(t, w.ProcessedAt.IsZero())
	}

	require.Len(t, withdrawals, 2)
	none, err := s.ListWithdrawals(ctx, alice.ID+1000)
	require.NoError(t, err)
	assert.Empty(t, none)
}
