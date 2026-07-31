// Package pgtest provisions isolated PostgreSQL databases for Parcel
// integration tests. It runs the real migration plan, so a test proves the
// shipped SQL rather than a hand written fixture schema.
package pgtest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// DSNVariable carries the administrative connection string. It is only read
// from the environment and never logged.
const DSNVariable = "IDP_PARCEL_POSTGRES_DSN"

var databaseSequence atomic.Uint64

// Pool returns a connection pool to a freshly migrated, test owned database.
// The database is dropped when the test finishes.
//
// Without a DSN the test skips locally, but CI fails: a required integration
// gate must never be silently absent from a pipeline.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminDSN := os.Getenv(DSNVariable)
	if adminDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is required in CI; the PostgreSQL gate must not be skipped", DSNVariable)
		}
		t.Skipf("%s is not set; skipping the PostgreSQL integration gate", DSNVariable)
	}

	ctx := t.Context()
	name := fmt.Sprintf("parcel_test_%d_%d", time.Now().UnixNano(), databaseSequence.Add(1))

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect as admin: %v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+quoteIdentifier(name)+` ENCODING 'UTF8'`); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create test database: %v", err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("close admin connection: %v", err)
	}

	testDSN, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("build test dsn: %v", err)
	}

	migrateConn, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Run(ctx, migrateConn, quiet); err != nil {
		t.Fatalf("apply migration plan: %v", err)
	}
	if err := migrateConn.Close(ctx); err != nil {
		t.Fatalf("close migration connection: %v", err)
	}

	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		dropDatabase(adminDSN, name)
	})
	return pool
}

func dropDatabase(adminDSN, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return
	}
	defer func() { _ = admin.Close(ctx) }()
	_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS `+quoteIdentifier(name)+` WITH (FORCE)`)
}

// withDatabase rewrites the database component of a DSN without parsing
// credentials into a log.
func withDatabase(dsn, name string) (string, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return "", err
	}
	config.Database = name

	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		config.Host, config.Port, config.User, config.Password, config.Database), nil
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
