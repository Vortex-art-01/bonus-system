package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/accrual"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

var errBoom = errors.New("boom")

type fakeStore struct {
	mu         sync.Mutex
	pending    []model.Order
	listErr    error
	updateErr  error
	processErr error
	updates    map[string]model.OrderStatus
	processed  map[string]float64
}

func newFakeStore(pending ...model.Order) *fakeStore {
	return &fakeStore{
		pending:   pending,
		updates:   map[string]model.OrderStatus{},
		processed: map[string]float64{},
	}
}

func (s *fakeStore) ListOrdersForProcessing(context.Context, int) ([]model.Order, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.pending, nil
}

func (s *fakeStore) UpdateOrderStatus(_ context.Context, number string, status model.OrderStatus) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates[number] = status
	return nil
}

func (s *fakeStore) ProcessOrder(_ context.Context, number string, accrual float64) error {
	if s.processErr != nil {
		return s.processErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[number] = accrual
	return nil
}

type fakeClient struct {
	responses map[string]*accrual.OrderInfo
	errs      map[string]error
	calls     atomic.Int32
}

func (c *fakeClient) GetOrder(ctx context.Context, number string) (*accrual.OrderInfo, error) {
	c.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err, ok := c.errs[number]; ok {
		return nil, err
	}
	if info, ok := c.responses[number]; ok {
		return info, nil
	}
	return nil, accrual.ErrOrderNotRegistered
}

func newWorker(store OrderStore, client AccrualClient, cfg Config) *Worker {
	return New(store, client, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
}

func order(number string, status model.OrderStatus) model.Order {
	return model.Order{Number: number, UserID: 1, Status: status}
}

func TestNew_Defaults(t *testing.T) {
	w := newWorker(newFakeStore(), &fakeClient{}, Config{})
	assert.Equal(t, time.Second, w.cfg.PollInterval)
	assert.Equal(t, 20, w.cfg.BatchSize)
	assert.Equal(t, 1, w.cfg.Concurrency)
}

func TestProcessBatch_AppliesStatuses(t *testing.T) {
	store := newFakeStore(
		order("registered", model.OrderStatusNew),
		order("processing", model.OrderStatusNew),
		order("already-processing", model.OrderStatusProcessing),
		order("invalid", model.OrderStatusProcessing),
		order("processed", model.OrderStatusProcessing),
		order("unknown-status", model.OrderStatusNew),
		order("not-registered", model.OrderStatusNew),
		order("failing", model.OrderStatusNew),
	)
	client := &fakeClient{
		responses: map[string]*accrual.OrderInfo{
			"registered":         {Order: "registered", Status: accrual.StatusRegistered},
			"processing":         {Order: "processing", Status: accrual.StatusProcessing},
			"already-processing": {Order: "already-processing", Status: accrual.StatusProcessing},
			"invalid":            {Order: "invalid", Status: accrual.StatusInvalid},
			"processed":          {Order: "processed", Status: accrual.StatusProcessed, Accrual: 729.98},
			"unknown-status":     {Order: "unknown-status", Status: "WEIRD"},
		},
		errs: map[string]error{"failing": errBoom},
	}

	w := newWorker(store, client, Config{Concurrency: 3})
	retryAfter := w.ProcessBatch(context.Background())

	assert.Zero(t, retryAfter)
	assert.Equal(t, map[string]model.OrderStatus{
		"registered": model.OrderStatusProcessing,
		"processing": model.OrderStatusProcessing,
		"invalid":    model.OrderStatusInvalid,
	}, store.updates)
	assert.Equal(t, map[string]float64{"processed": 729.98}, store.processed)
	assert.Equal(t, int32(8), client.calls.Load())
}

func TestProcessBatch_RateLimitStopsBatch(t *testing.T) {
	var orders []model.Order
	for _, n := range []string{"1", "2", "3", "4", "5", "6"} {
		orders = append(orders, order(n, model.OrderStatusNew))
	}
	store := newFakeStore(orders...)
	client := &fakeClient{
		errs: map[string]error{"1": &accrual.TooManyRequestsError{RetryAfter: 45 * time.Second}},
	}

	w := newWorker(store, client, Config{Concurrency: 1})
	retryAfter := w.ProcessBatch(context.Background())

	assert.Equal(t, 45*time.Second, retryAfter)
	assert.Less(t, client.calls.Load(), int32(6), "remaining orders must be skipped after a 429")
}

func TestProcessBatch_RateLimitKeepsLongestDelay(t *testing.T) {
	store := newFakeStore(order("1", model.OrderStatusNew), order("2", model.OrderStatusNew))
	client := &fakeClient{
		errs: map[string]error{
			"1": &accrual.TooManyRequestsError{RetryAfter: 10 * time.Second},
			"2": &accrual.TooManyRequestsError{RetryAfter: 30 * time.Second},
		},
	}

	w := newWorker(store, client, Config{Concurrency: 2})
	retryAfter := w.ProcessBatch(context.Background())

	assert.Contains(t, []time.Duration{10 * time.Second, 30 * time.Second}, retryAfter)
}

func TestProcessBatch_EmptyAndErrors(t *testing.T) {
	t.Run("nothing pending", func(t *testing.T) {
		client := &fakeClient{}
		w := newWorker(newFakeStore(), client, Config{})

		assert.Zero(t, w.ProcessBatch(context.Background()))
		assert.Zero(t, client.calls.Load())
	})

	t.Run("list error", func(t *testing.T) {
		store := newFakeStore()
		store.listErr = errBoom
		client := &fakeClient{}
		w := newWorker(store, client, Config{})

		assert.Zero(t, w.ProcessBatch(context.Background()))
		assert.Zero(t, client.calls.Load())
	})

	t.Run("store errors are swallowed", func(t *testing.T) {
		store := newFakeStore(order("a", model.OrderStatusNew), order("b", model.OrderStatusNew))
		store.updateErr = errBoom
		store.processErr = errBoom
		client := &fakeClient{responses: map[string]*accrual.OrderInfo{
			"a": {Order: "a", Status: accrual.StatusInvalid},
			"b": {Order: "b", Status: accrual.StatusProcessed, Accrual: 1},
		}}
		w := newWorker(store, client, Config{})

		assert.Zero(t, w.ProcessBatch(context.Background()))
		assert.Empty(t, store.updates)
		assert.Empty(t, store.processed)
	})

	t.Run("canceled context", func(t *testing.T) {
		store := newFakeStore(order("a", model.OrderStatusNew))
		client := &fakeClient{responses: map[string]*accrual.OrderInfo{
			"a": {Order: "a", Status: accrual.StatusInvalid},
		}}
		w := newWorker(store, client, Config{})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		assert.Zero(t, w.ProcessBatch(ctx))
		assert.Empty(t, store.updates)
	})
}

func TestRun_PollsUntilCanceled(t *testing.T) {
	store := newFakeStore(order("1", model.OrderStatusNew))
	client := &fakeClient{responses: map[string]*accrual.OrderInfo{
		"1": {Order: "1", Status: accrual.StatusProcessed, Accrual: 5},
	}}
	w := newWorker(store, client, Config{PollInterval: 5 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	require.Eventually(t, func() bool { return client.calls.Load() >= 3 }, time.Second, time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
	assert.Equal(t, map[string]float64{"1": 5}, store.processed)
}

func TestRun_BacksOffOnRateLimit(t *testing.T) {
	store := newFakeStore(order("1", model.OrderStatusNew))
	client := &fakeClient{errs: map[string]error{
		"1": &accrual.TooManyRequestsError{RetryAfter: time.Hour},
	}}
	w := newWorker(store, client, Config{PollInterval: time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(ctx)
	}()

	require.Eventually(t, func() bool { return client.calls.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(1), client.calls.Load(), "worker must wait for Retry-After before polling again")

	cancel()
	<-done
}
