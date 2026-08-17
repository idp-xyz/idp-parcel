package main

import (
	"context"
	"errors"
	"strings"
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

// Covers: ADR-0049 第四条与 dispatch.Config 的「零值不可用」——部署形态参数一个默认
// 都不给。缺一项就点名它停下：一个没人调过的节奏在生产上表现为「事件好像卡住了」，
// 而现场看不出那个数是谁定的。
func TestEveryDeploymentSettingIsRequiredByName(t *testing.T) {
	complete := map[string]string{
		"IDP_PARCEL_POSTGRES_DSN":              "postgres://parcel@127.0.0.1:5432/parcel",
		"IDP_PARCEL_ROUTE_SERVICE_PURPOSE":     "NETWORK_SERVICE",
		"IDP_PARCEL_DISPATCH_DELIVERY_TIMEOUT": "5s",
		"IDP_PARCEL_DISPATCH_LEASE":            "1m",
		"IDP_PARCEL_DISPATCH_RETRY_AFTER":      "30s",
		"IDP_PARCEL_DISPATCH_LIMIT":            "10",
		"IDP_PARCEL_DISPATCH_MAX_ATTEMPTS":     "5",
	}
	if _, err := settingsFromEnv(lookupIn(complete)); err != nil {
		t.Fatalf("齐全的配置反被拒：%v", err)
	}

	for missing := range complete {
		partial := map[string]string{}
		for name, value := range complete {
			if name != missing {
				partial[name] = value
			}
		}
		_, err := settingsFromEnv(lookupIn(partial))
		if err == nil {
			t.Fatalf("缺 %s 仍然装配出了配置——某处补了默认值", missing)
		}
		if !strings.Contains(err.Error(), missing) {
			t.Fatalf("缺 %s 时错误没有点名它：%v", missing, err)
		}
	}
}

func lookupIn(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}
