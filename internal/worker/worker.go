package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Vortex-art-01/bonus-system/internal/accrual"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type OrderStore interface {
	ListOrdersForProcessing(ctx context.Context, limit int) ([]model.Order, error)
	UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus) error
	ProcessOrder(ctx context.Context, number string, accrual model.Money) error
}

type AccrualClient interface {
	GetOrder(ctx context.Context, number string) (*accrual.OrderInfo, error)
}

type Config struct {
	PollInterval time.Duration
	BatchSize    int
	Concurrency  int
}

type Worker struct {
	store  OrderStore
	client AccrualClient
	log    *slog.Logger
	cfg    Config
}

func New(store OrderStore, client AccrualClient, log *slog.Logger, cfg Config) *Worker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	return &Worker{store: store, client: client, log: log, cfg: cfg}
}

func (w *Worker) Run(ctx context.Context) {
	w.log.Info("order worker started",
		slog.Duration("poll_interval", w.cfg.PollInterval),
		slog.Int("batch_size", w.cfg.BatchSize),
		slog.Int("concurrency", w.cfg.Concurrency),
	)
	defer w.log.Info("order worker stopped")

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		delay := w.cfg.PollInterval
		if retryAfter := w.ProcessBatch(ctx); retryAfter > delay {
			w.log.Warn("accrual system rate limit reached, pausing", slog.Duration("retry_after", retryAfter))
			delay = retryAfter
		}
		timer.Reset(delay)
	}
}

func (w *Worker) ProcessBatch(ctx context.Context) time.Duration {
	orders, err := w.store.ListOrdersForProcessing(ctx, w.cfg.BatchSize)
	if err != nil {
		if ctx.Err() == nil {
			w.log.Error("list pending orders", slog.Any("error", err))
		}
		return 0
	}
	if len(orders) == 0 {
		return 0
	}

	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu         sync.Mutex
		retryAfter time.Duration
		wg         sync.WaitGroup
		sem        = make(chan struct{}, w.cfg.Concurrency)
	)
loop:
	for _, order := range orders {
		select {
		case sem <- struct{}{}:
		case <-batchCtx.Done():
			break loop
		}

		wg.Go(func() {
			defer func() { <-sem }()

			if d := w.processOrder(batchCtx, order); d > 0 {
				mu.Lock()
				retryAfter = max(retryAfter, d)
				mu.Unlock()
				cancel()
			}
		})
	}
	wg.Wait()

	return retryAfter
}

func (w *Worker) processOrder(ctx context.Context, order model.Order) time.Duration {
	if ctx.Err() != nil {
		return 0
	}
	log := w.log.With(slog.String("order", order.Number))

	info, err := w.client.GetOrder(ctx, order.Number)
	var tooMany *accrual.TooManyRequestsError
	switch {
	case errors.As(err, &tooMany):
		return tooMany.RetryAfter
	case errors.Is(err, accrual.ErrOrderNotRegistered):
		return 0
	case err != nil:
		if ctx.Err() == nil {
			log.Error("query accrual system", slog.Any("error", err))
		}
		return 0
	}

	switch info.Status {
	case accrual.StatusRegistered, accrual.StatusProcessing:
		if order.Status == model.OrderStatusProcessing {
			return 0
		}
		err = w.store.UpdateOrderStatus(ctx, order.Number, model.OrderStatusProcessing)
	case accrual.StatusInvalid:
		err = w.store.UpdateOrderStatus(ctx, order.Number, model.OrderStatusInvalid)
	case accrual.StatusProcessed:
		err = w.store.ProcessOrder(ctx, order.Number, info.Accrual)
	default:
		log.Warn("unknown accrual status", slog.String("status", string(info.Status)))
		return 0
	}
	if err != nil {
		if ctx.Err() == nil {
			log.Error("store accrual result", slog.String("status", string(info.Status)), slog.Any("error", err))
		}
		return 0
	}

	log.Info("order updated", slog.String("status", string(info.Status)), slog.String("accrual", info.Accrual.String()))
	return 0
}
