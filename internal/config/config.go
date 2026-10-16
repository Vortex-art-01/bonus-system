package config

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRunAddress         = "localhost:8080"
	DefaultTokenTTL           = 24 * time.Hour
	DefaultWorkerPollInterval = time.Second
	DefaultWorkerBatchSize    = 20
	DefaultWorkerConcurrency  = 5
	DefaultShutdownTimeout    = 10 * time.Second
)

type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
	JWTSecret            string
	TokenTTL             time.Duration
	WorkerPollInterval   time.Duration
	WorkerBatchSize      int
	WorkerConcurrency    int
	ShutdownTimeout      time.Duration
}

type LookupEnvFunc func(key string) (string, bool)

func Load(args []string, lookupEnv LookupEnvFunc) (*Config, error) {
	cfg := &Config{
		RunAddress:         DefaultRunAddress,
		TokenTTL:           DefaultTokenTTL,
		WorkerPollInterval: DefaultWorkerPollInterval,
		WorkerBatchSize:    DefaultWorkerBatchSize,
		WorkerConcurrency:  DefaultWorkerConcurrency,
		ShutdownTimeout:    DefaultShutdownTimeout,
	}

	fs := flag.NewFlagSet("gophermart", flag.ContinueOnError)
	fs.StringVar(&cfg.RunAddress, "a", cfg.RunAddress, "address and port to listen on")
	fs.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "PostgreSQL connection URI")
	fs.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "accrual system address")
	fs.StringVar(&cfg.JWTSecret, "s", cfg.JWTSecret, "secret key for signing authentication tokens")
	fs.DurationVar(&cfg.TokenTTL, "token-ttl", cfg.TokenTTL, "lifetime of an authentication token")
	fs.DurationVar(&cfg.WorkerPollInterval, "poll-interval", cfg.WorkerPollInterval, "pause between polls of pending orders")
	fs.IntVar(&cfg.WorkerBatchSize, "batch-size", cfg.WorkerBatchSize, "maximum number of orders fetched per poll")
	fs.IntVar(&cfg.WorkerConcurrency, "concurrency", cfg.WorkerConcurrency, "number of concurrent requests to the accrual system")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	env := envReader{lookup: lookupEnv}
	env.str(&cfg.RunAddress, "RUN_ADDRESS")
	env.str(&cfg.DatabaseURI, "DATABASE_URI")
	env.str(&cfg.AccrualSystemAddress, "ACCRUAL_SYSTEM_ADDRESS")
	env.str(&cfg.JWTSecret, "JWT_SECRET")
	env.duration(&cfg.TokenTTL, "TOKEN_TTL")
	env.duration(&cfg.WorkerPollInterval, "WORKER_POLL_INTERVAL")
	env.integer(&cfg.WorkerBatchSize, "WORKER_BATCH_SIZE")
	env.integer(&cfg.WorkerConcurrency, "WORKER_CONCURRENCY")
	env.duration(&cfg.ShutdownTimeout, "SHUTDOWN_TIMEOUT")
	if env.err != nil {
		return nil, env.err
	}

	cfg.AccrualSystemAddress = normalizeURL(cfg.AccrualSystemAddress)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var errs []error
	if c.RunAddress == "" {
		errs = append(errs, errors.New("run address is required"))
	}
	if c.DatabaseURI == "" {
		errs = append(errs, errors.New("database URI is required"))
	}
	if c.TokenTTL <= 0 {
		errs = append(errs, errors.New("token TTL must be positive"))
	}
	if c.WorkerPollInterval <= 0 {
		errs = append(errs, errors.New("worker poll interval must be positive"))
	}
	if c.WorkerBatchSize <= 0 {
		errs = append(errs, errors.New("worker batch size must be positive"))
	}
	if c.WorkerConcurrency <= 0 {
		errs = append(errs, errors.New("worker concurrency must be positive"))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown timeout must be positive"))
	}
	return errors.Join(errs...)
}

func normalizeURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

type envReader struct {
	lookup LookupEnvFunc
	err    error
}

func (r *envReader) str(dst *string, key string) {
	if v, ok := r.lookup(key); ok {
		*dst = v
	}
}

func (r *envReader) duration(dst *time.Duration, key string) {
	v, ok := r.lookup(key)
	if !ok || r.err != nil {
		return
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.err = fmt.Errorf("parse %s: %w", key, err)
		return
	}
	*dst = d
}

func (r *envReader) integer(dst *int, key string) {
	v, ok := r.lookup(key)
	if !ok || r.err != nil {
		return
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.err = fmt.Errorf("parse %s: %w", key, err)
		return
	}
	*dst = n
}
