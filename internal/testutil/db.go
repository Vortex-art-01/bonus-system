package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Vortex-art-01/bonus-system/internal/storage/postgres"
)

const EnvDatabaseURI = "TEST_DATABASE_URI"

func DatabaseURI(t testing.TB, schema string) string {
	t.Helper()

	dsn := os.Getenv(EnvDatabaseURI)
	if dsn == "" {
		t.Skipf("%s is not set, skipping integration test", EnvDatabaseURI)
	}

	u, err := url.Parse(dsn)
	require.NoError(t, err, "parse %s", EnvDatabaseURI)

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err, "connect to test database")
	defer conn.Close(ctx)

	ident := pgx.Identifier{schema}.Sanitize()
	_, err = conn.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", ident))
	require.NoError(t, err, "drop schema")
	_, err = conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", ident))
	require.NoError(t, err, "create schema")

	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func NewPool(t testing.TB, schema string) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, DatabaseURI(t, schema))
	require.NoError(t, err, "create pool")
	t.Cleanup(pool.Close)

	require.NoError(t, postgres.Migrate(ctx, pool), "apply migrations")
	return pool
}
