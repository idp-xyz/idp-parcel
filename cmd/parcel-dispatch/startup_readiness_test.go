package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// 本文件只证装配点的「启动时就坏」这一格：库不可达或框架 schema 未施加时，
// assembleDispatcher 带原因失败，而不是交出一个起得来的进程。
//
// discardLogger 让本文件不去管失败观察口（ADR-0095）：这里证的是启动就绪，与投递失败
// 怎么出声无关，而真让它往测试输出里打日志只会淹掉用例自己的信号。传 nil 也能过，但那走的
// 是「没有观察口」那一支——装配点仍应收到一个真 logger，才与生产形状一致。
//
// 与 Loop 那一格刻意分开，两者不是同一件事。Loop 对一拍失败只记不停是对的——运行
// 中的依赖抖动不该拖死进程；但部署错误若也经那条路径表现，就与抖动在日志里长成同一
// 种东西，而两者要运维做的事相反：一个去改部署，一个去等或查上游。

// startupEnv 造一份除 DSN 外全部合法的部署形态。取包里的环境变量常量而不是重抄
// 字面量：抄错一个名字会让用例停在「必填项缺失」上，那与就绪检查无关，却一样是红的。
func startupEnv(dsn string) func(string) string {
	values := map[string]string{
		envDatabaseDSN:     dsn,
		envServicePurpose:  "NETWORK_SERVICE",
		envDeliveryTimeout: "5s",
		envLeaseFor:        "1m",
		envRetryAfter:      "30s",
		envBatchLimit:      "10",
		envMaxAttempts:     "5",
	}
	return func(name string) string { return values[name] }
}

// migratedDatabase 建一个空库并施加真实迁移计划，交回它的连接串。
//
// pgtest.Pool 交回的是连接池而不是 DSN，而装配点要的恰好是 DSN——它自己建池正是
// 被测的那一段，从外面递一个已建好的池进去会把要证的东西绕过去。
func migratedDatabase(t *testing.T) string {
	t.Helper()

	dsn := pgtest.FreshDatabase(t)
	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	if err := migrate.Run(t.Context(), conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}
	if err := conn.Close(t.Context()); err != nil {
		t.Fatalf("关闭迁移连接：%v", err)
	}
	return dsn
}

// Covers: 库不可达时装配失败。建池是惰性的，pgxpool.New 在这里不会报错——没有 Ping
// 这一步，错的 DSN 会让进程照常起来并常驻，以每拍报错的方式表现。
//
// 本用例不需要真实 PostgreSQL：要证的就是连不上，因此它在没有 DSN 的环境里照跑。
func TestAssemblyFailsWhenTheDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	// 端口 1 是特权端口，正常宿主上无人监听，拨号立即被拒。超时只兜住万一被占的
	// 情形，免得用例挂在拨号上而不是给出结论。
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	beat, cleanup, err := assembleDispatcher(
		ctx, startupEnv("postgres://parcel:parcel@127.0.0.1:1/postgres?sslmode=disable"), discardLogger())
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatalf("库不可达却装配成功了，交回了一拍：%v", beat)
	}
}

// Covers: 库可达但框架 schema 未施加时装配失败。CheckSchema 是 bento 现成的只读检查，
// 此前一次都没被调用过——缺 outbox/inbox 的库照样装配得出派发器，之后每一拍都在报
// 关系不存在。
func TestAssemblyFailsWhenTheFrameworkSchemaIsMissing(t *testing.T) {
	// FreshDatabase 建库但不施加任何迁移，正是「部署漏跑迁移」那一格。
	dsn := pgtest.FreshDatabase(t)

	beat, cleanup, err := assembleDispatcher(t.Context(), startupEnv(dsn), discardLogger())
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatalf("框架 schema 缺失却装配成功了，交回了一拍：%v", beat)
	}
	// 认 bento 自己的哨兵而不是比对错误串：串会随框架版本变，而这一格要证的正是那个
	// 只读检查真的跑过。换成「非 nil 即可」也不行——库不可达同样非 nil，两格会混。
	if !errors.Is(err, bentopg.ErrSchemaIncompatible) {
		t.Fatalf("错误未指向 schema 不兼容，就绪检查没走到 CheckSchema：%v", err)
	}
}

// Covers: 就绪检查不误伤健康部署。没有这一条，前两条可以被一个「永远返回错误」的
// 实现满足，而那种实现让进程一个都起不来。
func TestAssemblySucceedsOnAMigratedDatabase(t *testing.T) {
	dsn := migratedDatabase(t)

	beat, cleanup, err := assembleDispatcher(t.Context(), startupEnv(dsn), discardLogger())
	if err != nil {
		t.Fatalf("已施加迁移的库上装配失败：%v", err)
	}
	t.Cleanup(cleanup)
	if beat == nil {
		t.Fatal("装配成功却没有交回一拍")
	}
}
