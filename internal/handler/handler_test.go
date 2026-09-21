package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/middleware"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

var errBoom = errors.New("boom")

const (
	validToken = "valid-token"
	testUserID = int64(42)
)

type fakeParser struct{}

func (fakeParser) Parse(token string) (int64, error) {
	if token == validToken {
		return testUserID, nil
	}
	return 0, errors.New("bad token")
}

type fakeServices struct {
	register        func(ctx context.Context, login, password string) (string, error)
	login           func(ctx context.Context, login, password string) (string, error)
	upload          func(ctx context.Context, userID int64, number string) error
	list            func(ctx context.Context, userID int64) ([]model.Order, error)
	get             func(ctx context.Context, userID int64) (*model.Balance, error)
	withdraw        func(ctx context.Context, userID int64, order string, sum float64) error
	listWithdrawals func(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

func (f *fakeServices) Register(ctx context.Context, login, password string) (string, error) {
	return f.register(ctx, login, password)
}

func (f *fakeServices) Login(ctx context.Context, login, password string) (string, error) {
	return f.login(ctx, login, password)
}

func (f *fakeServices) Upload(ctx context.Context, userID int64, number string) error {
	return f.upload(ctx, userID, number)
}

func (f *fakeServices) List(ctx context.Context, userID int64) ([]model.Order, error) {
	return f.list(ctx, userID)
}

func (f *fakeServices) Get(ctx context.Context, userID int64) (*model.Balance, error) {
	return f.get(ctx, userID)
}

func (f *fakeServices) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	return f.withdraw(ctx, userID, order, sum)
}

func (f *fakeServices) ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return f.listWithdrawals(ctx, userID)
}

func newRouter(svc *fakeServices) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(svc, svc, svc, log).Router(fakeParser{})
}

type request struct {
	method      string
	path        string
	body        string
	contentType string
	auth        bool
}

func do(t *testing.T, h http.Handler, req request) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(req.method, req.path, strings.NewReader(req.body))
	if req.contentType != "" {
		r.Header.Set("Content-Type", req.contentType)
	}
	if req.auth {
		r.Header.Set("Authorization", "Bearer "+validToken)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		register   func(ctx context.Context, login, password string) (string, error)
		wantStatus int
	}{
		{
			name: "ok",
			body: `{"login":"alice","password":"secret"}`,
			register: func(_ context.Context, login, password string) (string, error) {
				assert.Equal(t, "alice", login)
				assert.Equal(t, "secret", password)
				return "tok", nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "login taken",
			body:       `{"login":"alice","password":"secret"}`,
			register:   func(context.Context, string, string) (string, error) { return "", model.ErrLoginTaken },
			wantStatus: http.StatusConflict,
		},
		{
			name:       "internal error",
			body:       `{"login":"alice","password":"secret"}`,
			register:   func(context.Context, string, string) (string, error) { return "", errBoom },
			wantStatus: http.StatusInternalServerError,
		},
		{name: "malformed json", body: `{"login":`, wantStatus: http.StatusBadRequest},
		{name: "empty login", body: `{"login":"","password":"secret"}`, wantStatus: http.StatusBadRequest},
		{name: "empty password", body: `{"login":"alice","password":""}`, wantStatus: http.StatusBadRequest},
		{name: "login too long", body: `{"login":"` + strings.Repeat("a", 65) + `","password":"x"}`, wantStatus: http.StatusBadRequest},
		{name: "password too long", body: `{"login":"alice","password":"` + strings.Repeat("a", 73) + `"}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeServices{register: tt.register}
			if svc.register == nil {
				svc.register = func(context.Context, string, string) (string, error) {
					t.Fatal("service must not be called")
					return "", nil
				}
			}

			w := do(t, newRouter(svc), request{method: http.MethodPost, path: "/api/user/register", body: tt.body, contentType: "application/json"})

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, "Bearer tok", w.Header().Get("Authorization"))
				cookies := w.Result().Cookies()
				require.Len(t, cookies, 1)
				assert.Equal(t, middleware.CookieName, cookies[0].Name)
				assert.Equal(t, "tok", cookies[0].Value)
				assert.True(t, cookies[0].HttpOnly)
			} else {
				assert.Empty(t, w.Header().Get("Authorization"))
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		login      func(ctx context.Context, login, password string) (string, error)
		wantStatus int
	}{
		{
			name:       "ok",
			body:       `{"login":"alice","password":"secret"}`,
			login:      func(context.Context, string, string) (string, error) { return "tok", nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong credentials",
			body:       `{"login":"alice","password":"secret"}`,
			login:      func(context.Context, string, string) (string, error) { return "", model.ErrInvalidCredentials },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "internal error",
			body:       `{"login":"alice","password":"secret"}`,
			login:      func(context.Context, string, string) (string, error) { return "", errBoom },
			wantStatus: http.StatusInternalServerError,
		},
		{name: "malformed json", body: `not json`, wantStatus: http.StatusBadRequest},
		{name: "missing fields", body: `{}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeServices{login: tt.login}
			if svc.login == nil {
				svc.login = func(context.Context, string, string) (string, error) {
					t.Fatal("service must not be called")
					return "", nil
				}
			}

			w := do(t, newRouter(svc), request{method: http.MethodPost, path: "/api/user/login", body: tt.body, contentType: "application/json"})

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, "Bearer tok", w.Header().Get("Authorization"))
			}
		})
	}
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	h := newRouter(&fakeServices{})
	routes := []request{
		{method: http.MethodPost, path: "/api/user/orders", body: "12345678903"},
		{method: http.MethodGet, path: "/api/user/orders"},
		{method: http.MethodGet, path: "/api/user/balance"},
		{method: http.MethodPost, path: "/api/user/balance/withdraw", body: `{"order":"2377225624","sum":1}`},
		{method: http.MethodGet, path: "/api/user/withdrawals"},
	}

	for _, req := range routes {
		t.Run(req.method+" "+req.path, func(t *testing.T) {
			w := do(t, h, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestUploadOrder(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		upload     func(ctx context.Context, userID int64, number string) error
		wantStatus int
	}{
		{
			name: "accepted",
			body: "12345678903",
			upload: func(_ context.Context, userID int64, number string) error {
				assert.Equal(t, testUserID, userID)
				assert.Equal(t, "12345678903", number)
				return nil
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "trims whitespace",
			body: "  12345678903\n",
			upload: func(_ context.Context, _ int64, number string) error {
				assert.Equal(t, "12345678903", number)
				return nil
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "already uploaded by user",
			body:       "12345678903",
			upload:     func(context.Context, int64, string) error { return model.ErrOrderAlreadyUploaded },
			wantStatus: http.StatusOK,
		},
		{
			name:       "uploaded by another user",
			body:       "12345678903",
			upload:     func(context.Context, int64, string) error { return model.ErrOrderUploadedByAnother },
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid number",
			body:       "12345678904",
			upload:     func(context.Context, int64, string) error { return model.ErrInvalidOrderNumber },
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "internal error",
			body:       "12345678903",
			upload:     func(context.Context, int64, string) error { return errBoom },
			wantStatus: http.StatusInternalServerError,
		},
		{name: "empty body", body: "  ", wantStatus: http.StatusBadRequest},
		{name: "body too large", body: strings.Repeat("1", maxOrderNumberSize+1), wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeServices{upload: tt.upload}
			if svc.upload == nil {
				svc.upload = func(context.Context, int64, string) error {
					t.Fatal("service must not be called")
					return nil
				}
			}

			w := do(t, newRouter(svc), request{method: http.MethodPost, path: "/api/user/orders", body: tt.body, contentType: "text/plain", auth: true})

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestListOrders(t *testing.T) {
	uploaded := time.Date(2020, 12, 10, 15, 15, 45, 0, time.FixedZone("MSK", 3*3600))
	accrual := 500.0

	t.Run("ok", func(t *testing.T) {
		svc := &fakeServices{list: func(_ context.Context, userID int64) ([]model.Order, error) {
			assert.Equal(t, testUserID, userID)
			return []model.Order{
				{Number: "9278923470", Status: model.OrderStatusProcessed, Accrual: &accrual, UploadedAt: uploaded},
				{Number: "12345678903", Status: model.OrderStatusProcessing, UploadedAt: uploaded.Add(-time.Minute)},
			}, nil
		}}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/orders", auth: true})

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.JSONEq(t, `[
			{"number":"9278923470","status":"PROCESSED","accrual":500,"uploaded_at":"2020-12-10T15:15:45+03:00"},
			{"number":"12345678903","status":"PROCESSING","uploaded_at":"2020-12-10T15:14:45+03:00"}
		]`, w.Body.String())
	})

	t.Run("no content", func(t *testing.T) {
		svc := &fakeServices{list: func(context.Context, int64) ([]model.Order, error) { return nil, nil }}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/orders", auth: true})

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("internal error", func(t *testing.T) {
		svc := &fakeServices{list: func(context.Context, int64) ([]model.Order, error) { return nil, errBoom }}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/orders", auth: true})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestGetBalance(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		svc := &fakeServices{get: func(_ context.Context, userID int64) (*model.Balance, error) {
			assert.Equal(t, testUserID, userID)
			return &model.Balance{Current: 500.5, Withdrawn: 42}, nil
		}}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/balance", auth: true})

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"current":500.5,"withdrawn":42}`, w.Body.String())
	})

	t.Run("internal error", func(t *testing.T) {
		svc := &fakeServices{get: func(context.Context, int64) (*model.Balance, error) { return nil, errBoom }}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/balance", auth: true})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestWithdraw(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		withdraw   func(ctx context.Context, userID int64, order string, sum float64) error
		wantStatus int
	}{
		{
			name: "ok",
			body: `{"order":"2377225624","sum":751}`,
			withdraw: func(_ context.Context, userID int64, order string, sum float64) error {
				assert.Equal(t, testUserID, userID)
				assert.Equal(t, "2377225624", order)
				assert.Equal(t, 751.0, sum)
				return nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "insufficient funds",
			body:       `{"order":"2377225624","sum":751}`,
			withdraw:   func(context.Context, int64, string, float64) error { return model.ErrInsufficientFunds },
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "invalid order",
			body:       `{"order":"1","sum":751}`,
			withdraw:   func(context.Context, int64, string, float64) error { return model.ErrInvalidOrderNumber },
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "invalid sum",
			body:       `{"order":"2377225624","sum":0}`,
			withdraw:   func(context.Context, int64, string, float64) error { return model.ErrInvalidWithdrawalSum },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "internal error",
			body:       `{"order":"2377225624","sum":751}`,
			withdraw:   func(context.Context, int64, string, float64) error { return errBoom },
			wantStatus: http.StatusInternalServerError,
		},
		{name: "malformed json", body: `{"order":`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeServices{withdraw: tt.withdraw}
			if svc.withdraw == nil {
				svc.withdraw = func(context.Context, int64, string, float64) error {
					t.Fatal("service must not be called")
					return nil
				}
			}

			w := do(t, newRouter(svc), request{method: http.MethodPost, path: "/api/user/balance/withdraw", body: tt.body, contentType: "application/json", auth: true})

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestListWithdrawals(t *testing.T) {
	processed := time.Date(2020, 12, 9, 16, 9, 57, 0, time.FixedZone("MSK", 3*3600))

	t.Run("ok", func(t *testing.T) {
		svc := &fakeServices{listWithdrawals: func(_ context.Context, userID int64) ([]model.Withdrawal, error) {
			assert.Equal(t, testUserID, userID)
			return []model.Withdrawal{{ID: 1, UserID: userID, Order: "2377225624", Sum: 500, ProcessedAt: processed}}, nil
		}}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/withdrawals", auth: true})

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `[{"order":"2377225624","sum":500,"processed_at":"2020-12-09T16:09:57+03:00"}]`, w.Body.String())
	})

	t.Run("no content", func(t *testing.T) {
		svc := &fakeServices{listWithdrawals: func(context.Context, int64) ([]model.Withdrawal, error) { return []model.Withdrawal{}, nil }}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/withdrawals", auth: true})

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("internal error", func(t *testing.T) {
		svc := &fakeServices{listWithdrawals: func(context.Context, int64) ([]model.Withdrawal, error) { return nil, errBoom }}

		w := do(t, newRouter(svc), request{method: http.MethodGet, path: "/api/user/withdrawals", auth: true})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestGzipRoundTrip(t *testing.T) {
	svc := &fakeServices{get: func(context.Context, int64) (*model.Balance, error) {
		return &model.Balance{Current: 1, Withdrawn: 2}, nil
	}}
	svc.register = func(context.Context, string, string) (string, error) { return "tok", nil }
	h := newRouter(svc)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(`{"login":"alice","password":"secret"}`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	r := httptest.NewRequest(http.MethodPost, "/api/user/register", &buf)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)

	r = httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	r.Header.Set("Authorization", "Bearer "+validToken)
	r.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	zr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.JSONEq(t, `{"current":1,"withdrawn":2}`, string(body))
}

func TestUserIDMissingFromContext(t *testing.T) {
	h := New(nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for name, fn := range map[string]http.HandlerFunc{
		"uploadOrder":     h.uploadOrder,
		"listOrders":      h.listOrders,
		"getBalance":      h.getBalance,
		"withdraw":        h.withdraw,
		"listWithdrawals": h.listWithdrawals,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			fn(w, httptest.NewRequest(http.MethodGet, "/", nil))
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestUnknownRoute(t *testing.T) {
	w := do(t, newRouter(&fakeServices{}), request{method: http.MethodGet, path: "/api/nothing"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}
