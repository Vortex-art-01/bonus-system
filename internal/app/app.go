package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vortex-art-01/bonus-system/internal/accrual"
	"github.com/Vortex-art-01/bonus-system/internal/auth"
	"github.com/Vortex-art-01/bonus-system/internal/config"
	"github.com/Vortex-art-01/bonus-system/internal/handler"
	"github.com/Vortex-art-01/bonus-system/internal/service"
	"github.com/Vortex-art-01/bonus-system/internal/storage/postgres"
	"github.com/Vortex-art-01/bonus-system/internal/worker"
)

const readHeaderTimeout = 10 * time.Second

func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	store := postgres.New(pool)

	secret := cfg.JWTSecret
	if secret == "" {
		if secret, err = auth.GenerateSecret(); err != nil {
			return err
		}
		log.Warn("JWT_SECRET is not set: using a random secret, tokens will not survive a restart")
	}
	tokens := auth.NewTokenManager(secret, cfg.TokenTTL)

	h := handler.New(
		service.NewUserService(store, tokens),
		service.NewOrderService(store),
		service.NewBalanceService(store),
		log,
	)
	srv := &http.Server{
		Addr:              cfg.RunAddress,
		Handler:           h.Router(tokens),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	var wg sync.WaitGroup
	if cfg.AccrualSystemAddress == "" {
		log.Warn("ACCRUAL_SYSTEM_ADDRESS is not set: orders will stay in NEW status")
	} else {
		w := worker.New(store, accrual.New(cfg.AccrualSystemAddress), log, worker.Config{
			PollInterval: cfg.WorkerPollInterval,
			BatchSize:    cfg.WorkerBatchSize,
			Concurrency:  cfg.WorkerConcurrency,
		})
		wg.Go(func() { w.Run(ctx) })
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server started", slog.String("addr", cfg.RunAddress))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			runErr = fmt.Errorf("http server: %w", err)
		}
	}
	cancel()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = fmt.Errorf("shutdown http server: %w", err)
	}
	wg.Wait()

	log.Info("service stopped")
	return runErr
}
