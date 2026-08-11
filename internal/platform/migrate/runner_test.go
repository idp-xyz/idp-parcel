package migrate_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件取 `PBC-06` 的证据：框架迁移按 checksum 渲染，并在真实 PostgreSQL 16 上
// 通过 `CheckSchema`。用真实数据库而非替身，是因为要证的恰好是随产品发出的那份
// SQL 在真实引擎上成立；一个内存替身能让所有用例都绿，却什么也没证。

func TestFrameworkMigrationsApplyAndSchemaCheckPasses(t *testing.T) {
	dsn := pgtest.FreshDatabase(t)
	ctx := t.Context()

	conn := connect(t, dsn)
	if err := migrate.Run(ctx, conn, quietLogger()); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("打开连接池：%v", err)
	}
	t.Cleanup(pool.Close)

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	// `CheckSchema` 是只读验证：它比对真实库里的对象子集与框架期望的形状。
	// 迁移施加完它仍不过，说明渲染出的 SQL 与框架自己的期望不是一回事。
	if err := db.CheckSchema(ctx); err != nil {
		t.Fatalf("迁移后 CheckSchema 未通过：%v", err)
	}
}

// TestHistoryRecordsTheFrameworkArtifactIdentity 证历史表记下的是可追回不可变工件
// 的那几项，而不只是「跑过了」。缺了 checksum 与框架版本，一个已部署的库就无法与
// 某个精确候选对上。
func TestHistoryRecordsTheFrameworkArtifactIdentity(t *testing.T) {
	dsn := pgtest.FreshDatabase(t)
	ctx := t.Context()

	conn := connect(t, dsn)
	if err := migrate.Run(ctx, conn, quietLogger()); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}

	expected := make(map[string]string)
	for _, asset := range bentopg.Migrations() {
		expected["framework/"+asset.ID] = asset.Checksum
	}
	if len(expected) == 0 {
		t.Fatal("框架未提供迁移资产；本用例会空过")
	}

	rows, err := conn.Query(ctx,
		`SELECT migration_id, canonical_checksum, framework_version, target_schema
		   FROM `+migrate.SchemaHistory+`.applied_migration`)
	if err != nil {
		t.Fatalf("读迁移历史：%v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var id, checksum, schema string
		var frameworkVersion *string
		if err := rows.Scan(&id, &checksum, &frameworkVersion, &schema); err != nil {
			t.Fatalf("扫描迁移历史：%v", err)
		}
		want, known := expected[id]
		if !known {
			t.Errorf("历史里出现计划外的迁移 %q", id)
			continue
		}
		if checksum != want {
			t.Errorf("%s 记录的 checksum 为 %q，框架清单为 %q", id, checksum, want)
		}
		if frameworkVersion == nil || *frameworkVersion != migrate.FrameworkVersion {
			t.Errorf("%s 记录的框架版本为 %v，应为 %q", id, frameworkVersion, migrate.FrameworkVersion)
		}
		if schema != migrate.SchemaBento {
			t.Errorf("%s 记录的 schema 为 %q，应为 %q", id, schema, migrate.SchemaBento)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读迁移历史：%v", err)
	}
	if seen != len(expected) {
		t.Errorf("历史记录 %d 条，框架清单 %d 条", seen, len(expected))
	}
}

// TestRunningTwiceAppliesNothingNew 证重跑不重放。部署窗口里重试是常态，第二次
// 若把同一批 DDL 再施加一遍，轻则报错重则改坏已有对象。
func TestRunningTwiceAppliesNothingNew(t *testing.T) {
	dsn := pgtest.FreshDatabase(t)
	ctx := t.Context()

	conn := connect(t, dsn)
	if err := migrate.Run(ctx, conn, quietLogger()); err != nil {
		t.Fatalf("首次施加：%v", err)
	}
	first := countHistory(t, conn)

	if err := migrate.Run(ctx, conn, quietLogger()); err != nil {
		t.Fatalf("再次施加：%v", err)
	}
	if second := countHistory(t, conn); second != first {
		t.Errorf("重跑后历史由 %d 条变为 %d 条；重跑不应新增记录", first, second)
	}
}

// TestChecksumDriftBlocksTheRun 证被改写过的模板会被挡住。这条是 `PBC-06`
// 「按 checksum 渲染」的负向面：没有它，一份被复制改写的框架 SQL 可以照常施加，
// 而历史表还会声称它来自那个不可变候选。
func TestChecksumDriftBlocksTheRun(t *testing.T) {
	dsn := pgtest.FreshDatabase(t)
	ctx := t.Context()

	conn := connect(t, dsn)
	if err := migrate.Run(ctx, conn, quietLogger()); err != nil {
		t.Fatalf("首次施加：%v", err)
	}

	// 直接篡改历史里记下的校验和，等价于「已施加的那份工件与当前构建携带的不是
	// 同一份」。
	if _, err := conn.Exec(ctx,
		`UPDATE `+migrate.SchemaHistory+`.applied_migration
		    SET canonical_checksum = 'sha256:tampered'`); err != nil {
		t.Fatalf("篡改历史校验和：%v", err)
	}

	err := migrate.Run(ctx, conn, quietLogger())
	if !errors.Is(err, migrate.ErrChecksumDrift) {
		t.Fatalf("校验和漂移后应以 ErrChecksumDrift 阻断，实得：%v", err)
	}
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	return conn
}

func countHistory(t *testing.T, conn *pgx.Conn) int {
	t.Helper()

	var count int
	if err := conn.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaHistory+`.applied_migration`).Scan(&count); err != nil {
		t.Fatalf("统计迁移历史：%v", err)
	}
	return count
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
