package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envFrom(m map[string]string) LookupEnvFunc {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load([]string{"-d", "postgres://db"}, envFrom(nil))
	require.NoError(t, err)

	assert.Equal(t, DefaultRunAddress, cfg.RunAddress)
	assert.Equal(t, "postgres://db", cfg.DatabaseURI)
	assert.Empty(t, cfg.AccrualSystemAddress)
	assert.Empty(t, cfg.JWTSecret)
	assert.Equal(t, DefaultTokenTTL, cfg.TokenTTL)
	assert.Equal(t, DefaultWorkerPollInterval, cfg.WorkerPollInterval)
	assert.Equal(t, DefaultWorkerBatchSize, cfg.WorkerBatchSize)
	assert.Equal(t, DefaultWorkerConcurrency, cfg.WorkerConcurrency)
	assert.Equal(t, DefaultShutdownTimeout, cfg.ShutdownTimeout)
}

func TestLoad_Flags(t *testing.T) {
	args := []string{
		"-a", ":9000",
		"-d", "postgres://flag",
		"-r", "localhost:8081",
		"-s", "secret",
		"-token-ttl", "1h",
		"-poll-interval", "2s",
		"-batch-size", "7",
		"-concurrency", "3",
		"-shutdown-timeout", "3s",
	}
	cfg, err := Load(args, envFrom(nil))
	require.NoError(t, err)

	assert.Equal(t, ":9000", cfg.RunAddress)
	assert.Equal(t, "postgres://flag", cfg.DatabaseURI)
	assert.Equal(t, "http://localhost:8081", cfg.AccrualSystemAddress)
	assert.Equal(t, "secret", cfg.JWTSecret)
	assert.Equal(t, time.Hour, cfg.TokenTTL)
	assert.Equal(t, 2*time.Second, cfg.WorkerPollInterval)
	assert.Equal(t, 7, cfg.WorkerBatchSize)
	assert.Equal(t, 3, cfg.WorkerConcurrency)
	assert.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
}

func TestLoad_EnvOverridesFlags(t *testing.T) {
	env := envFrom(map[string]string{
		"RUN_ADDRESS":            ":7000",
		"DATABASE_URI":           "postgres://env",
		"ACCRUAL_SYSTEM_ADDRESS": "https://accrual.example.com/",
		"JWT_SECRET":             "env-secret",
		"TOKEN_TTL":              "30m",
		"WORKER_POLL_INTERVAL":   "5s",
		"WORKER_BATCH_SIZE":      "50",
		"WORKER_CONCURRENCY":     "2",
		"SHUTDOWN_TIMEOUT":       "1s",
	})
	args := []string{"-a", ":9000", "-d", "postgres://flag", "-r", "flag:1", "-s", "flag-secret"}

	cfg, err := Load(args, env)
	require.NoError(t, err)

	assert.Equal(t, ":7000", cfg.RunAddress)
	assert.Equal(t, "postgres://env", cfg.DatabaseURI)
	assert.Equal(t, "https://accrual.example.com", cfg.AccrualSystemAddress)
	assert.Equal(t, "env-secret", cfg.JWTSecret)
	assert.Equal(t, 30*time.Minute, cfg.TokenTTL)
	assert.Equal(t, 5*time.Second, cfg.WorkerPollInterval)
	assert.Equal(t, 50, cfg.WorkerBatchSize)
	assert.Equal(t, 2, cfg.WorkerConcurrency)
	assert.Equal(t, time.Second, cfg.ShutdownTimeout)
}

func TestLoad_Errors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		msg  string
	}{
		{name: "missing database", args: nil, msg: "database URI is required"},
		{name: "empty run address", args: []string{"-a", "", "-d", "x"}, msg: "run address is required"},
		{name: "unknown flag", args: []string{"-zzz"}, msg: "parse flags"},
		{name: "bad duration env", args: []string{"-d", "x"}, env: map[string]string{"TOKEN_TTL": "soon"}, msg: "parse TOKEN_TTL"},
		{name: "bad integer env", args: []string{"-d", "x"}, env: map[string]string{"WORKER_BATCH_SIZE": "many"}, msg: "parse WORKER_BATCH_SIZE"},
		{name: "non-positive batch", args: []string{"-d", "x", "-batch-size", "0"}, msg: "batch size must be positive"},
		{name: "non-positive concurrency", args: []string{"-d", "x", "-concurrency", "-1"}, msg: "concurrency must be positive"},
		{name: "non-positive ttl", args: []string{"-d", "x", "-token-ttl", "0s"}, msg: "token TTL must be positive"},
		{name: "non-positive poll", args: []string{"-d", "x", "-poll-interval", "0s"}, msg: "poll interval must be positive"},
		{name: "non-positive shutdown", args: []string{"-d", "x", "-shutdown-timeout", "0s"}, msg: "shutdown timeout must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.args, envFrom(tt.env))
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tt.msg)
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := map[string]string{
		"":                             "",
		"   ":                          "",
		"localhost:8080":               "http://localhost:8080",
		"localhost:8080/":              "http://localhost:8080",
		"http://localhost:8080":        "http://localhost:8080",
		"https://accrual.example.com/": "https://accrual.example.com",
	}
	for in, want := range tests {
		assert.Equal(t, want, normalizeURL(in), "input %q", in)
	}
}
