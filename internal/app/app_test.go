package app_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/app"
	"github.com/Vortex-art-01/bonus-system/internal/config"
	"github.com/Vortex-art-01/bonus-system/internal/model"
	"github.com/Vortex-art-01/bonus-system/internal/testutil"
)

type accrualStub struct {
	mu     sync.Mutex
	orders map[string]map[string]any
}

func (a *accrualStub) set(number string, payload map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.orders[number] = payload
}

func (a *accrualStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	number := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	a.mu.Lock()
	payload, ok := a.orders[number]
	a.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().String()
}

type client struct {
	t     *testing.T
	base  string
	http  *http.Client
	token string
}

type response struct {
	code   int
	header http.Header
	body   string
}

func (c *client) do(method, path, body string, headers map[string]string) response {
	c.t.Helper()
	req, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	require.NoError(c.t, err)
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(resp.Body)
		require.NoError(c.t, err)
		reader = zr
	}
	data, err := io.ReadAll(reader)
	require.NoError(c.t, err)
	return response{code: resp.StatusCode, header: resp.Header, body: string(data)}
}

func (c *client) json(method, path, body string) response {
	return c.do(method, path, body, map[string]string{"Content-Type": "application/json"})
}

func (c *client) text(method, path, body string) response {
	return c.do(method, path, body, map[string]string{"Content-Type": "text/plain"})
}

func waitReady(t *testing.T, base string) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/api/user/orders")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusUnauthorized
	}, 10*time.Second, 50*time.Millisecond, "server did not start")
}

type orderView struct {
	Number     string       `json:"number"`
	Status     string       `json:"status"`
	Accrual    *model.Money `json:"accrual"`
	UploadedAt string       `json:"uploaded_at"`
}

type balanceView struct {
	Current   model.Money `json:"current"`
	Withdrawn model.Money `json:"withdrawn"`
}

type withdrawalView struct {
	Order       string      `json:"order"`
	Sum         model.Money `json:"sum"`
	ProcessedAt string      `json:"processed_at"`
}

func TestIntegration_EndToEnd(t *testing.T) {
	dsn := testutil.DatabaseURI(t, "gophermart_test_app")

	accrual := &accrualStub{orders: map[string]map[string]any{}}
	accrualSrv := httptest.NewServer(accrual)
	defer accrualSrv.Close()

	addr := freeAddr(t)
	cfg := &config.Config{
		RunAddress:           addr,
		DatabaseURI:          dsn,
		AccrualSystemAddress: accrualSrv.URL,
		JWTSecret:            "integration-test-secret",
		TokenTTL:             time.Hour,
		WorkerPollInterval:   50 * time.Millisecond,
		WorkerBatchSize:      10,
		WorkerConcurrency:    2,
		ShutdownTimeout:      5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()

	base := "http://" + addr
	waitReady(t, base)
	alice := &client{t: t, base: base, http: &http.Client{Timeout: 5 * time.Second}}
	bob := &client{t: t, base: base, http: alice.http}

	resp := alice.json(http.MethodPost, "/api/user/register", `{"login":"alice","password":"secret"}`)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	require.True(t, strings.HasPrefix(resp.header.Get("Authorization"), "Bearer "))
	alice.token = resp.header.Get("Authorization")

	resp = alice.json(http.MethodPost, "/api/user/register", `{"login":"alice","password":"other"}`)
	assert.Equal(t, http.StatusConflict, resp.code)

	resp = bob.json(http.MethodPost, "/api/user/login", `{"login":"alice","password":"wrong"}`)
	assert.Equal(t, http.StatusUnauthorized, resp.code)

	resp = bob.json(http.MethodPost, "/api/user/register", `{"login":"bob","password":"secret"}`)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	resp = bob.json(http.MethodPost, "/api/user/login", `{"login":"bob","password":"secret"}`)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	bob.token = resp.header.Get("Authorization")

	resp = alice.do(http.MethodGet, "/api/user/orders", "", nil)
	assert.Equal(t, http.StatusNoContent, resp.code)
	resp = alice.do(http.MethodGet, "/api/user/withdrawals", "", nil)
	assert.Equal(t, http.StatusNoContent, resp.code)

	resp = alice.text(http.MethodPost, "/api/user/orders", "12345678903")
	assert.Equal(t, http.StatusAccepted, resp.code, resp.body)
	resp = alice.text(http.MethodPost, "/api/user/orders", "12345678903")
	assert.Equal(t, http.StatusOK, resp.code)
	resp = bob.text(http.MethodPost, "/api/user/orders", "12345678903")
	assert.Equal(t, http.StatusConflict, resp.code)
	resp = alice.text(http.MethodPost, "/api/user/orders", "12345678904")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.code)
	resp = alice.text(http.MethodPost, "/api/user/orders", "")
	assert.Equal(t, http.StatusBadRequest, resp.code)
	resp = alice.text(http.MethodPost, "/api/user/orders", "9278923470")
	assert.Equal(t, http.StatusAccepted, resp.code, resp.body)

	resp = alice.do(http.MethodGet, "/api/user/orders", "", nil)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var orders []orderView
	require.NoError(t, json.Unmarshal([]byte(resp.body), &orders))
	require.Len(t, orders, 2)
	assert.Equal(t, "9278923470", orders[0].Number, "newest first")
	assert.Equal(t, "NEW", orders[0].Status)
	assert.Nil(t, orders[0].Accrual)
	_, err := time.Parse(time.RFC3339, orders[0].UploadedAt)
	assert.NoError(t, err)

	resp = alice.json(http.MethodPost, "/api/user/balance/withdraw", `{"order":"2377225624","sum":10}`)
	assert.Equal(t, http.StatusPaymentRequired, resp.code)

	accrual.set("12345678903", map[string]any{"order": "12345678903", "status": "PROCESSED", "accrual": 729.98})
	accrual.set("9278923470", map[string]any{"order": "9278923470", "status": "INVALID"})

	require.Eventually(t, func() bool {
		resp := alice.do(http.MethodGet, "/api/user/orders", "", nil)
		if resp.code != http.StatusOK {
			return false
		}
		var orders []orderView
		if err := json.Unmarshal([]byte(resp.body), &orders); err != nil || len(orders) != 2 {
			return false
		}
		return orders[0].Status == "INVALID" && orders[1].Status == "PROCESSED"
	}, 10*time.Second, 50*time.Millisecond, "worker did not process orders")

	resp = alice.do(http.MethodGet, "/api/user/orders", "", nil)
	require.NoError(t, json.Unmarshal([]byte(resp.body), &orders))
	assert.Nil(t, orders[0].Accrual)
	require.NotNil(t, orders[1].Accrual)
	assert.Equal(t, model.Money(72998), *orders[1].Accrual)

	resp = alice.do(http.MethodGet, "/api/user/balance", "", nil)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var balance balanceView
	require.NoError(t, json.Unmarshal([]byte(resp.body), &balance))
	assert.Equal(t, model.Money(72998), balance.Current)
	assert.Equal(t, model.Money(0), balance.Withdrawn)

	resp = alice.json(http.MethodPost, "/api/user/balance/withdraw", `{"order":"2377225624","sum":29.98}`)
	assert.Equal(t, http.StatusOK, resp.code, resp.body)
	resp = alice.json(http.MethodPost, "/api/user/balance/withdraw", `{"order":"2377225625","sum":1}`)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.code)
	resp = alice.json(http.MethodPost, "/api/user/balance/withdraw", `{"order":"79927398713","sum":1000}`)
	assert.Equal(t, http.StatusPaymentRequired, resp.code)

	resp = alice.do(http.MethodGet, "/api/user/balance", "", nil)
	require.NoError(t, json.Unmarshal([]byte(resp.body), &balance))
	assert.Equal(t, model.Money(70000), balance.Current)
	assert.Equal(t, model.Money(2998), balance.Withdrawn)

	resp = alice.do(http.MethodGet, "/api/user/withdrawals", "", nil)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var withdrawals []withdrawalView
	require.NoError(t, json.Unmarshal([]byte(resp.body), &withdrawals))
	require.Len(t, withdrawals, 1)
	assert.Equal(t, "2377225624", withdrawals[0].Order)
	assert.Equal(t, model.Money(2998), withdrawals[0].Sum)
	_, err = time.Parse(time.RFC3339, withdrawals[0].ProcessedAt)
	assert.NoError(t, err)

	resp = bob.do(http.MethodGet, "/api/user/orders", "", nil)
	assert.Equal(t, http.StatusNoContent, resp.code)
	resp = bob.do(http.MethodGet, "/api/user/withdrawals", "", nil)
	assert.Equal(t, http.StatusNoContent, resp.code)
	resp = bob.do(http.MethodGet, "/api/user/balance", "", nil)
	require.NoError(t, json.Unmarshal([]byte(resp.body), &balance))
	assert.Zero(t, balance.Current)

	resp = alice.do(http.MethodGet, "/api/user/orders", "", map[string]string{"Accept-Encoding": "gzip"})
	assert.Equal(t, http.StatusOK, resp.code)
	assert.Equal(t, "gzip", resp.header.Get("Content-Encoding"))
	require.NoError(t, json.Unmarshal([]byte(resp.body), &orders))
	assert.Len(t, orders, 2)

	anon := &client{t: t, base: base, http: alice.http}
	resp = anon.do(http.MethodGet, "/api/user/balance", "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.code)
	anon.token = "Bearer forged"
	resp = anon.do(http.MethodGet, "/api/user/balance", "", nil)
	assert.Equal(t, http.StatusUnauthorized, resp.code)

	cancel()
	select {
	case err := <-runErr:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("service did not stop")
	}
}

func TestIntegration_Run_BadDatabase(t *testing.T) {
	cfg := &config.Config{
		RunAddress:      freeAddr(t),
		DatabaseURI:     "postgres://nobody:nothing@127.0.0.1:1/nowhere?sslmode=disable&connect_timeout=1",
		TokenTTL:        time.Hour,
		ShutdownTimeout: time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := app.Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database")
}
