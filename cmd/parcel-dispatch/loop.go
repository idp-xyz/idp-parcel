package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Beat 是派发的一拍。进程循环只认 DispatchOnce，好让测试替身与未来的
// *dispatch.Dispatcher 都能由 main 注入。
type Beat interface {
	DispatchOnce(ctx context.Context) (int, error)
}

// Loop 是派发进程的循环骨架：按间隔调一拍，ctx 取消即停。一拍失败只记下来，
// 不把进程拖死——重试节奏在下一拍，停机信号在 ctx。
type Loop struct {
	beat     Beat
	interval time.Duration
	logger   *slog.Logger
}

func NewLoop(beat Beat, interval time.Duration, logger *slog.Logger) (*Loop, error) {
	if beat == nil {
		return nil, errors.New("parcel-dispatch: beat is required")
	}
	if interval <= 0 {
		return nil, errors.New("parcel-dispatch: interval must be positive")
	}
	return &Loop{beat: beat, interval: interval, logger: logger}, nil
}

// Run 反复执行一拍直到 ctx 取消。取消后不再拍。返回 nil 表示正常停机。
func (loop *Loop) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := loop.beat.DispatchOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			loop.report(err)
		}
		if err := waitInterval(ctx, loop.interval); err != nil {
			return nil
		}
	}
}

func (loop *Loop) report(err error) {
	if loop.logger == nil {
		return
	}
	loop.logger.Error("dispatch beat failed", "error", err)
}

func waitInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func intervalFromEnv(getenv func(string) string) (time.Duration, error) {
	raw := getenv("IDP_PARCEL_DISPATCH_INTERVAL")
	if raw == "" {
		return 0, errors.New("parcel-dispatch: IDP_PARCEL_DISPATCH_INTERVAL is required")
	}
	interval, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parcel-dispatch: IDP_PARCEL_DISPATCH_INTERVAL: %w", err)
	}
	if interval <= 0 {
		return 0, errors.New("parcel-dispatch: IDP_PARCEL_DISPATCH_INTERVAL must be positive")
	}
	return interval, nil
}
