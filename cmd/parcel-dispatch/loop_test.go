package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type beatStub struct {
	fn func(context.Context) (int, error)
}

func (stub beatStub) DispatchOnce(ctx context.Context) (int, error) {
	return stub.fn(ctx)
}

func TestLoopDoesNotBeatAfterCancel(t *testing.T) {
	var beats atomic.Int32
	ctx, cancel := context.WithCancel(t.Context())
	loop, err := NewLoop(beatStub{fn: func(context.Context) (int, error) {
		beats.Add(1)
		cancel()
		return 1, nil
	}}, time.Hour, nil)
	if err != nil {
		t.Fatalf("构造循环：%v", err)
	}

	if err := loop.Run(ctx); err != nil {
		t.Fatalf("停机应返回 nil，实得：%v", err)
	}
	if got := beats.Load(); got != 1 {
		t.Fatalf("取消后仍在拍：beats=%d，want 1", got)
	}
}

func TestLoopSurvivesAFailedBeat(t *testing.T) {
	var beats atomic.Int32
	ctx, cancel := context.WithCancel(t.Context())
	loop, err := NewLoop(beatStub{fn: func(context.Context) (int, error) {
		n := beats.Add(1)
		if n == 1 {
			return 0, errors.New("这一拍失败")
		}
		cancel()
		return 1, nil
	}}, time.Millisecond, nil)
	if err != nil {
		t.Fatalf("构造循环：%v", err)
	}

	if err := loop.Run(ctx); err != nil {
		t.Fatalf("失败一拍不得拖死循环，实得：%v", err)
	}
	if got := beats.Load(); got != 2 {
		t.Fatalf("失败后没有下一拍：beats=%d，want 2", got)
	}
}

func TestAssembleDispatcherIsNotWired(t *testing.T) {
	beat, err := assembleDispatcher()
	if !errors.Is(err, errDispatcherNotWired) || beat != nil {
		t.Fatalf("组合根未完成时应交回 errDispatcherNotWired，实得 beat=%v err=%v", beat, err)
	}
}
