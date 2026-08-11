// Package pgtest 为 Parcel 的集成测试准备彼此隔离的 PostgreSQL 数据库。它跑的是
// 真实迁移计划，因此测试证的是随产品发出的那份 SQL，而不是一份手写的夹具 schema。
package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// DSNVariable 携带管理连接串。只从环境读取，且从不写进日志。
const DSNVariable = "IDP_PARCEL_POSTGRES_DSN"

var databaseSequence atomic.Uint64

// uniqueDatabaseName 造一个在整个 PostgreSQL 实例里唯一的库名。
//
// 只用「时间戳 + 进程内计数器」不够：Go 为每个包单独起一个测试进程，计数器因而
// 各自从头开始，而 Windows 的 `UnixNano` 分辨率粗到两个几乎同时启动的进程会拿到
// 同一个值——两个包并行跑时就会撞出 `duplicate key ... pg_database_datname_index`。
// 加进程号消掉同刻不同进程那一半，再加随机后缀兜住同进程内的极端情形。
func uniqueDatabaseName(t *testing.T) string {
	t.Helper()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("生成测试库名随机后缀：%v", err)
	}
	return fmt.Sprintf("parcel_test_%d_%d_%s",
		os.Getpid(), databaseSequence.Add(1), hex.EncodeToString(suffix))
}

// Pool 返回一个连向全新数据库的连接池，该库已施加真实迁移计划，测试结束时删除。
//
// 没有 DSN 时本地跳过，但 CI 失败：一道必需的集成门禁绝不能在流水线里静默缺席。
// 跳过与通过必须长得不一样，否则「没跑」会被读成「跑过了」。
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminDSN := os.Getenv(DSNVariable)
	if adminDSN == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("CI 中必须设置 %s；PostgreSQL 门禁不得跳过", DSNVariable)
		}
		t.Skipf("未设置 %s，跳过 PostgreSQL 集成门禁", DSNVariable)
	}

	ctx := t.Context()
	name := uniqueDatabaseName(t)

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("以管理身份连接失败：%v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+quoteIdentifier(name)+` ENCODING 'UTF8'`); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("创建测试库失败：%v", err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("关闭管理连接失败：%v", err)
	}

	testDSN, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("构造测试库连接串失败：%v", err)
	}

	migrateConn, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatalf("连接测试库失败：%v", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Run(ctx, migrateConn, quiet); err != nil {
		t.Fatalf("施加迁移计划失败：%v", err)
	}
	if err := migrateConn.Close(ctx); err != nil {
		t.Fatalf("关闭迁移连接失败：%v", err)
	}

	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("打开连接池失败：%v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		dropDatabase(adminDSN, name)
	})
	return pool
}

// AdminDSN 返回管理连接串，供需要自己掌管连接生命周期的用例使用；未设置时与
// Pool 采取同样的跳过／失败分界。
func AdminDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv(DSNVariable)
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("CI 中必须设置 %s；PostgreSQL 门禁不得跳过", DSNVariable)
		}
		t.Skipf("未设置 %s，跳过 PostgreSQL 集成门禁", DSNVariable)
	}
	return dsn
}

// FreshDatabase 创建一个空的测试库并返回其连接串，**不**施加任何迁移。
// 迁移执行器自身的用例需要从空库起步，否则证不到「第一次施加」这条路径。
func FreshDatabase(t *testing.T) string {
	t.Helper()

	adminDSN := AdminDSN(t)
	ctx := t.Context()
	name := uniqueDatabaseName(t)

	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("以管理身份连接失败：%v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+quoteIdentifier(name)+` ENCODING 'UTF8'`); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("创建测试库失败：%v", err)
	}
	if err := admin.Close(ctx); err != nil {
		t.Fatalf("关闭管理连接失败：%v", err)
	}

	testDSN, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("构造测试库连接串失败：%v", err)
	}
	t.Cleanup(func() { dropDatabase(adminDSN, name) })
	return testDSN
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

// withDatabase 改写连接串的库名部分，过程中不把凭据带进任何日志。
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
