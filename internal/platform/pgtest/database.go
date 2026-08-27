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
	"sync"
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

// 管理面（建库/删库）走每测试进程一条常驻连接，互斥串行。
//
// 动机不是省时间而是省端口：Windows 把主动关闭的 TCP 连接压在 TIME_WAIT 里占用
// 动态端口，逐用例新建管理连接时单包实测就产出六百余条指向 :55432 的 TIME_WAIT，
// 多包并行足以耗尽端口预算（WSAEADDRINUSE）。四类连接里只有管理连接可以跨用例
// 复用而不破坏「每测试一个物理库」的隔离语义——它只跑 CREATE/DROP DATABASE，
// 不携带任何测试库内状态。
var (
	adminMu   sync.Mutex
	adminConn *pgx.Conn
)

// runAsAdmin 在进程级共享的管理连接上执行 op。
//
// 共享连接必须用 Background 建立：挂在任何一个用例的 t.Context() 上，那个用例
// 结束时的取消会把之后所有用例的管理面一起带走。每次操作各自限时，免得一条
// 卡死的管理语句拖住整个进程的建库/删库队列。
func runAsAdmin(adminDSN string, op func(ctx context.Context, conn *pgx.Conn) error) error {
	adminMu.Lock()
	defer adminMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if adminConn == nil || adminConn.IsClosed() {
		conn, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			return fmt.Errorf("以管理身份连接：%w", err)
		}
		adminConn = conn
	}
	if err := op(ctx, adminConn); err != nil {
		// 这里分不清语句级失败（库名冲突之类，连接还好好的）与连接级失败
		// （网络断、协议错乱）。Ping 一次，坏了就弃掉让下次重建，别让一条
		// 坏连接把后续所有管理操作拖成同一种假错。
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer pingCancel()
		if adminConn.Ping(pingCtx) != nil {
			_ = adminConn.Close(pingCtx)
			adminConn = nil
		}
		return err
	}
	return nil
}

func createDatabase(adminDSN, name string) error {
	return runAsAdmin(adminDSN, func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `CREATE DATABASE `+quoteIdentifier(name)+` ENCODING 'UTF8'`)
		return err
	})
}

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

	if err := createDatabase(adminDSN, name); err != nil {
		t.Fatalf("创建测试库失败：%v", err)
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

	// pgxpool 默认按 CPU 数开连接，测试用例的并发度用不满它，尖峰时却成倍放大
	// TIME_WAIT。封顶 2 保留「一条在事务里、一条旁路观察」的余量；真需要更高
	// 并发的用例应自己掌管连接（AdminDSN 那条路），不抬这里的默认。
	pool, err := pgxpool.New(ctx, testDSN+" pool_max_conns=2")
	if err != nil {
		t.Fatalf("打开连接池失败：%v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		dropDatabase(t, adminDSN, name)
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
	name := uniqueDatabaseName(t)

	if err := createDatabase(adminDSN, name); err != nil {
		t.Fatalf("创建测试库失败：%v", err)
	}

	testDSN, err := withDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("构造测试库连接串失败：%v", err)
	}
	t.Cleanup(func() { dropDatabase(t, adminDSN, name) })
	return testDSN
}

// dropDatabase 删除测试库，失败必须出声：此前连不上就静默 return，孤儿库会无声
// 积累到没人记得来历。出声不等于清扫——清扫要按「无活跃 backend + 建立时间足够旧」
// 双条件另行处理，这里只保证失败可见。
func dropDatabase(t *testing.T, adminDSN, name string) {
	t.Helper()

	if err := runAsAdmin(adminDSN, func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+quoteIdentifier(name)+` WITH (FORCE)`)
		return err
	}); err != nil {
		t.Errorf("删除测试库 %s 失败，将遗留孤儿库：%v", name, err)
	}
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
