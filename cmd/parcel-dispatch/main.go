package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, os.Getenv); err != nil {
		logger.Error("Parcel dispatch stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, getenv func(string) string) error {
	interval, err := intervalFromEnv(getenv)
	if err != nil {
		return err
	}
	beat, err := assembleDispatcher()
	if err != nil {
		return err
	}
	loop, err := NewLoop(beat, interval, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return loop.Run(ctx)
}
