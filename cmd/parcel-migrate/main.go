// Command parcel-migrate applies the Parcel migration plan explicitly. It is
// run before a deployment by an operator or a deployment job; no application
// process may execute DDL, and production API and outbox roles do not hold the
// privileges this command needs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const connectTimeout = 15 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	plan := flag.Bool("plan", false, "print the ordered migration plan and exit without connecting")
	flag.Parse()

	if *plan {
		return printPlan()
	}

	// The DSN carries credentials, so it is only accepted through the
	// environment and never echoed.
	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		return errors.New("IDP_PARCEL_POSTGRES_DSN is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	conn, err := pgx.Connect(connectCtx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), connectTimeout)
		defer closeCancel()
		_ = conn.Close(closeCtx)
	}()

	return migrate.Run(ctx, conn, logger)
}

func printPlan() error {
	steps, err := migrate.Plan()
	if err != nil {
		return err
	}
	for index, step := range steps {
		version := step.FrameworkVersion
		if version == "" {
			version = "-"
		}
		fmt.Printf("%3d  %-10s  %-18s  %-45s  %s  %s\n",
			index+1, step.Origin, step.Schema, step.ID, version, step.Checksum)
	}
	return nil
}
