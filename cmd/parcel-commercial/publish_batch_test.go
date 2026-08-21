package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证进程口的批推进（syn-wall-door-audit 票 03 件 3）。
//
// 它补的是 application 层 TestAConflictingItemDoesNotRetractAnEarlierSavedItem 拿不到
// 的东西：那条走登记册替身，证不了写进去的行真的在库里。本条端到端跑 runPublish——
// 前一项的行必须真已提交，否则后一项那次同对象异正文根本撞不出 CONTENT_CONFLICT，
// 断言因此不可能空过。
//
// **它守不住事务边界，别指望**：AT-PC-011 的「逐项独立成败」在本进程口靠的是循环里
// 每项一个事务，而**冲突不是错误**——把循环整个包进一个事务，第二项照样判冲突、
// 事务照样提交，本条依旧绿。真要钉住那个结构，得让后一项以技术失败收场再看前一项
// 还在不在，而技术失败今天只来自基础设施故障，从批文里造不出来。这一格没有守门人，
// 照实记在此处与票 03。

// freshMigratedDSN 建一个空库、施加真实迁移计划，交回连接串。
//
// 不能用 pgtest.Pool：它交回的是池，而进程口按连接串自己开池（openDatabase），
// 测试要能把同一个库喂给它。
func freshMigratedDSN(t *testing.T) string {
	t.Helper()
	dsn := pgtest.FreshDatabase(t)

	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Run(ctx, conn, quiet); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatalf("关闭迁移连接：%v", err)
	}
	return dsn
}

func batchFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写批文：%v", err)
	}
	return path
}

// rulePackageItem 造一份最小发布项：不带声明、生效边界已开，落库即取效。
func rulePackageItem(objectID, digest string) string {
	return `{
      "tenantId": "tenant-1",
      "kind": "ACCEPTANCE_RULE_PACKAGE",
      "objectId": "` + objectID + `",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "` + digest + `",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {
        "reference": "approval-` + objectID + `",
        "source": "source-` + objectID + `",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "CONFIRMED"
    }`
}

func countVersions(t *testing.T, dsn, objectID string) int {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.commercial_version
		  WHERE tenant_id = $1 AND object_id = $2`,
		"tenant-1", objectID,
	).Scan(&count); err != nil {
		t.Fatalf("统计版本行：%v", err)
	}
	return count
}

func runCLI(t *testing.T, dsn string, args ...string) int {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	}, &out, &errOut)
	t.Logf("stdout:\n%s", out.String())
	if errOut.Len() > 0 {
		t.Logf("stderr:\n%s", errOut.String())
	}
	return code
}

// Covers: AT-PC-011「发布批逐项独立成败」的进程口半边——同一批里后一项撞内容冲突，
// 不得把前一项已落库的发布带走。
func TestPublishBatchKeepsEarlierItemWhenALaterItemConflicts(t *testing.T) {
	dsn := freshMigratedDSN(t)

	// 先把 rules-conflict 按一份正文发出去，好让第二批里的同对象异正文撞上冲突。
	seeded := batchFile(t, `{"items":[`+rulePackageItem("rules-conflict", "sha256:original")+`]}`)
	if code := runCLI(t, dsn, "publish", "-input", seeded); code != exitLanded {
		t.Fatalf("铺垫批 exit = %d, want %d", code, exitLanded)
	}

	// 第二批两项：全新对象在前，异正文的同对象在后。
	batch := batchFile(t, `{"items":[`+
		rulePackageItem("rules-fresh", "sha256:fresh")+`,`+
		rulePackageItem("rules-conflict", "sha256:CHANGED")+
		`]}`)
	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitAttention {
		t.Fatalf("混合批 exit = %d, want %d（有冲突要报请人看）", code, exitAttention)
	}

	if n := countVersions(t, dsn, "rules-fresh"); n != 1 {
		t.Fatalf("rules-fresh 版本行 = %d, want 1——后一项冲突把前一项已落库的发布带走了", n)
	}
	if n := countVersions(t, dsn, "rules-conflict"); n != 1 {
		t.Fatalf("rules-conflict 版本行 = %d, want 1——冲突不得顶替也不得追加第二版", n)
	}
}
