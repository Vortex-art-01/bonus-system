package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/auth"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

var errBoom = errors.New("boom")

type fakeRepo struct {
	createUser      func(ctx context.Context, login, passwordHash string) (*model.User, error)
	getUserByLogin  func(ctx context.Context, login string) (*model.User, error)
	createOrder     func(ctx context.Context, number string, userID int64) error
	listOrders      func(ctx context.Context, userID int64) ([]model.Order, error)
	getBalance      func(ctx context.Context, userID int64) (*model.Balance, error)
	withdraw        func(ctx context.Context, userID int64, order string, sum float64) error
	listWithdrawals func(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

func (f *fakeRepo) CreateUser(ctx context.Context, login, passwordHash string) (*model.User, error) {
	return f.createUser(ctx, login, passwordHash)
}

func (f *fakeRepo) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	return f.getUserByLogin(ctx, login)
}

func (f *fakeRepo) CreateOrder(ctx context.Context, number string, userID int64) error {
	return f.createOrder(ctx, number, userID)
}

func (f *fakeRepo) ListOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	return f.listOrders(ctx, userID)
}

func (f *fakeRepo) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	return f.getBalance(ctx, userID)
}

func (f *fakeRepo) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	return f.withdraw(ctx, userID, order, sum)
}

func (f *fakeRepo) ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return f.listWithdrawals(ctx, userID)
}

type fakeTokens struct {
	issue func(userID int64) (string, error)
}

func (f *fakeTokens) Issue(userID int64) (string, error) { return f.issue(userID) }

func tokenFor(userID int64) (string, error) { return "token-" + string(rune('0'+userID)), nil }

func TestUserService_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		repo := &fakeRepo{
			createUser: func(_ context.Context, login, hash string) (*model.User, error) {
				assert.Equal(t, "alice", login)
				assert.True(t, auth.CheckPassword(hash, "pass"))
				return &model.User{ID: 1, Login: login}, nil
			},
		}
		svc := NewUserService(repo, &fakeTokens{issue: tokenFor})

		token, err := svc.Register(ctx, "alice", "pass")
		require.NoError(t, err)
		assert.Equal(t, "token-1", token)
	})

	t.Run("login taken", func(t *testing.T) {
		repo := &fakeRepo{
			createUser: func(context.Context, string, string) (*model.User, error) {
				return nil, model.ErrLoginTaken
			},
		}
		svc := NewUserService(repo, &fakeTokens{issue: tokenFor})

		_, err := svc.Register(ctx, "alice", "pass")
		assert.ErrorIs(t, err, model.ErrLoginTaken)
	})

	t.Run("password too long", func(t *testing.T) {
		svc := NewUserService(&fakeRepo{}, &fakeTokens{issue: tokenFor})

		_, err := svc.Register(ctx, "alice", strings.Repeat("x", auth.MaxPasswordLength+1))
		assert.Error(t, err)
	})

	t.Run("token error", func(t *testing.T) {
		repo := &fakeRepo{
			createUser: func(context.Context, string, string) (*model.User, error) {
				return &model.User{ID: 1}, nil
			},
		}
		svc := NewUserService(repo, &fakeTokens{issue: func(int64) (string, error) { return "", errBoom }})

		_, err := svc.Register(ctx, "alice", "pass")
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestUserService_Login(t *testing.T) {
	ctx := context.Background()
	hash, err := auth.HashPassword("pass")
	require.NoError(t, err)

	userWithHash := func(context.Context, string) (*model.User, error) {
		return &model.User{ID: 2, Login: "alice", PasswordHash: hash}, nil
	}

	t.Run("ok", func(t *testing.T) {
		svc := NewUserService(&fakeRepo{getUserByLogin: userWithHash}, &fakeTokens{issue: tokenFor})

		token, err := svc.Login(ctx, "alice", "pass")
		require.NoError(t, err)
		assert.Equal(t, "token-2", token)
	})

	t.Run("wrong password", func(t *testing.T) {
		svc := NewUserService(&fakeRepo{getUserByLogin: userWithHash}, &fakeTokens{issue: tokenFor})

		_, err := svc.Login(ctx, "alice", "wrong")
		assert.ErrorIs(t, err, model.ErrInvalidCredentials)
	})

	t.Run("unknown user", func(t *testing.T) {
		repo := &fakeRepo{
			getUserByLogin: func(context.Context, string) (*model.User, error) { return nil, model.ErrUserNotFound },
		}
		svc := NewUserService(repo, &fakeTokens{issue: tokenFor})

		_, err := svc.Login(ctx, "bob", "pass")
		assert.ErrorIs(t, err, model.ErrInvalidCredentials)
	})

	t.Run("repository error", func(t *testing.T) {
		repo := &fakeRepo{
			getUserByLogin: func(context.Context, string) (*model.User, error) { return nil, errBoom },
		}
		svc := NewUserService(repo, &fakeTokens{issue: tokenFor})

		_, err := svc.Login(ctx, "bob", "pass")
		assert.ErrorIs(t, err, errBoom)
		assert.NotErrorIs(t, err, model.ErrInvalidCredentials)
	})

	t.Run("token error", func(t *testing.T) {
		svc := NewUserService(&fakeRepo{getUserByLogin: userWithHash},
			&fakeTokens{issue: func(int64) (string, error) { return "", errBoom }})

		_, err := svc.Login(ctx, "alice", "pass")
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestOrderService_Upload(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		var gotNumber string
		var gotUser int64
		repo := &fakeRepo{createOrder: func(_ context.Context, number string, userID int64) error {
			gotNumber, gotUser = number, userID
			return nil
		}}

		require.NoError(t, NewOrderService(repo).Upload(ctx, 7, "12345678903"))
		assert.Equal(t, "12345678903", gotNumber)
		assert.Equal(t, int64(7), gotUser)
	})

	t.Run("invalid number", func(t *testing.T) {
		repo := &fakeRepo{createOrder: func(context.Context, string, int64) error {
			t.Fatal("repository must not be called")
			return nil
		}}

		err := NewOrderService(repo).Upload(ctx, 7, "12345678904")
		assert.ErrorIs(t, err, model.ErrInvalidOrderNumber)
	})

	t.Run("conflicts pass through", func(t *testing.T) {
		for _, want := range []error{model.ErrOrderAlreadyUploaded, model.ErrOrderUploadedByAnother, errBoom} {
			repo := &fakeRepo{createOrder: func(context.Context, string, int64) error { return want }}
			assert.ErrorIs(t, NewOrderService(repo).Upload(ctx, 7, "12345678903"), want)
		}
	})
}

func TestOrderService_List(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("ok", func(t *testing.T) {
		want := []model.Order{{Number: "1", UserID: 7, Status: model.OrderStatusNew, UploadedAt: now}}
		repo := &fakeRepo{listOrders: func(_ context.Context, userID int64) ([]model.Order, error) {
			assert.Equal(t, int64(7), userID)
			return want, nil
		}}

		got, err := NewOrderService(repo).List(ctx, 7)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("error", func(t *testing.T) {
		repo := &fakeRepo{listOrders: func(context.Context, int64) ([]model.Order, error) { return nil, errBoom }}

		_, err := NewOrderService(repo).List(ctx, 7)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestBalanceService_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		repo := &fakeRepo{getBalance: func(context.Context, int64) (*model.Balance, error) {
			return &model.Balance{Current: 500.5, Withdrawn: 42}, nil
		}}

		got, err := NewBalanceService(repo).Get(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, &model.Balance{Current: 500.5, Withdrawn: 42}, got)
	})

	t.Run("error", func(t *testing.T) {
		repo := &fakeRepo{getBalance: func(context.Context, int64) (*model.Balance, error) { return nil, errBoom }}

		_, err := NewBalanceService(repo).Get(ctx, 1)
		assert.ErrorIs(t, err, errBoom)
	})
}

func TestBalanceService_Withdraw(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		var called bool
		repo := &fakeRepo{withdraw: func(_ context.Context, userID int64, order string, sum float64) error {
			called = true
			assert.Equal(t, int64(1), userID)
			assert.Equal(t, "2377225624", order)
			assert.Equal(t, 751.0, sum)
			return nil
		}}

		require.NoError(t, NewBalanceService(repo).Withdraw(ctx, 1, "2377225624", 751))
		assert.True(t, called)
	})

	t.Run("validation", func(t *testing.T) {
		repo := &fakeRepo{withdraw: func(context.Context, int64, string, float64) error {
			t.Fatal("repository must not be called")
			return nil
		}}
		svc := NewBalanceService(repo)

		assert.ErrorIs(t, svc.Withdraw(ctx, 1, "2377225625", 10), model.ErrInvalidOrderNumber)
		assert.ErrorIs(t, svc.Withdraw(ctx, 1, "", 10), model.ErrInvalidOrderNumber)
		assert.ErrorIs(t, svc.Withdraw(ctx, 1, "2377225624", 0), model.ErrInvalidWithdrawalSum)
		assert.ErrorIs(t, svc.Withdraw(ctx, 1, "2377225624", -5), model.ErrInvalidWithdrawalSum)
	})

	t.Run("insufficient funds", func(t *testing.T) {
		repo := &fakeRepo{withdraw: func(context.Context, int64, string, float64) error { return model.ErrInsufficientFunds }}

		err := NewBalanceService(repo).Withdraw(ctx, 1, "2377225624", 751)
		assert.ErrorIs(t, err, model.ErrInsufficientFunds)
	})
}

func TestBalanceService_ListWithdrawals(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		want := []model.Withdrawal{{ID: 1, UserID: 1, Order: "2377225624", Sum: 500}}
		repo := &fakeRepo{listWithdrawals: func(context.Context, int64) ([]model.Withdrawal, error) { return want, nil }}

		got, err := NewBalanceService(repo).ListWithdrawals(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("error", func(t *testing.T) {
		repo := &fakeRepo{listWithdrawals: func(context.Context, int64) ([]model.Withdrawal, error) { return nil, errBoom }}

		_, err := NewBalanceService(repo).ListWithdrawals(ctx, 1)
		assert.ErrorIs(t, err, errBoom)
	})
}
