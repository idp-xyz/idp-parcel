package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

const (
	defaultAddress  = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("Parcel API stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	address := os.Getenv("IDP_PARCEL_HTTP_ADDR")
	if address == "" {
		address = defaultAddress
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 开池在起服务之前：部署坏了（DSN 缺失、库不可达、缺框架 schema）要让进程带原因
	// 退出，而不是先挂上监听端口再让每个请求各报一次错。ctx 由停机信号驱动，启动途中
	// 收到 SIGTERM 就当场停下。
	db, closeDB, err := openDatabase(ctx, os.Getenv)
	if err != nil {
		return err
	}
	defer closeDB()

	submission, err := buildSubmissionOrchestration(db)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewWithEndpoints(buildinfo.Current(), assembleBusinessEndpoints(submission)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveResult := make(chan error, 1)
	go func() {
		logger.Info("Parcel API listening", "address", address)
		serveResult <- server.ListenAndServe()
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveResult; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
