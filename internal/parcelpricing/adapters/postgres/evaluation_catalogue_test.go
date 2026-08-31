package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证评价登记册读面（票 admin-skeleton-closure-batch/03，
// 形状照 ADR-0077）：检索列面照登转写、快照列不透出、跨租户不可见、空租户答空列表、
// limit 生效且非正拒。
//
// 夹具以测试内 SQL 插行铺设（S 级合成行，SYN- 前缀）：评价是业务事实，写入方是渠道
// 墙后的评价编排，没有登记 CLI 可借用；经领域构造整份评价再 Save 属编排用例的测试
// 疆界，读面机制验证只需要「册上有行」这个事实本身。插行经过迁移钉住的全部 CHECK，
// 合规行不代表真实经营数据。
func newEvaluationCatalogue(t *testing.T) (*adapter.EvaluationCatalogue, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewEvaluationCatalogue(db)
	if err != nil {
		t.Fatalf("构造评价册读面：%v", err)
	}
	return catalogue, pool
}

// insertEvaluationRow 直插一行合规评价（见文件头：机制验证的夹具纪律；直用池 Exec
// 是 visibilityexception 读面测试的既有先例）。
func insertEvaluationRow(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	evaluationID, tenantID, status string,
	recordedAt time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_pricing.evaluation
			(evaluation_id, tenant_id, status, semantic_digest,
			 plan_content_digest, canonicalization, snapshot, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		evaluationID, tenantID, status,
		"sha256:syn-semantic-"+evaluationID,
		"sha256:syn-plan-"+evaluationID,
		"c14n-v1",
		[]byte(`{"synthetic":true}`),
		recordedAt,
	); err != nil {
		t.Fatalf("插评价行 %s：%v", evaluationID, err)
	}
}

// Covers: 评价册照列转写——状态、双摘要与规范化版本照登透出，快照不透出；排序按
// 登记时间倒序稳定可重复；他租的评价不进本租户列表，空租户答空。
func TestEvaluationCatalogueTranscribesTheColumnFace(t *testing.T) {
	catalogue, pool := newEvaluationCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-001", "tenant-a", "COMPLETED", base)
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-002", "tenant-a", "UNRATABLE", base.Add(time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-THEIRS", "tenant-b", "COMPLETED", base.Add(2*time.Hour))

	rows, err := catalogue.ListEvaluations(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列评价：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，want 2（他租的评价不得进本租户列表）", len(rows))
	}
	// 登记时间倒序：后登的 UNRATABLE 在前。
	if rows[0].EvaluationID != "SYN-EVAL-002" || rows[1].EvaluationID != "SYN-EVAL-001" {
		t.Fatalf("排序变形：%q, %q", rows[0].EvaluationID, rows[1].EvaluationID)
	}

	completed := rows[1]
	if completed.Status != "COMPLETED" ||
		completed.SemanticDigest != "sha256:syn-semantic-SYN-EVAL-001" ||
		completed.PlanContentDigest != "sha256:syn-plan-SYN-EVAL-001" ||
		completed.Canonicalization != "c14n-v1" {
		t.Fatalf("检索列面变形：%+v", completed)
	}
	if !completed.RecordedAt.Equal(base) {
		t.Fatalf("登记时间变形：%v", completed.RecordedAt)
	}
	if unratable := rows[0]; unratable.Status != "UNRATABLE" {
		t.Fatalf("失败态没照登透出：%+v", unratable)
	}

	empty, err := catalogue.ListEvaluations(ctx, evaluationValue(t, domain.NewTenantID, "tenant-empty"), 10)
	if err != nil {
		t.Fatalf("空租户上列：%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空租户答了 %d 行，want 0", len(empty))
	}
}

// Covers: ADR-0077 Decision 五——limit 生效、非正拒，与主数据目录两口同判据。
func TestEvaluationCatalogueAppliesTheLimitAndRejectsNonPositive(t *testing.T) {
	catalogue, pool := newEvaluationCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"SYN-EVAL-A", "SYN-EVAL-B", "SYN-EVAL-C"} {
		insertEvaluationRow(t, pool, ctx, id, "tenant-a", "PENDING", base.Add(time.Duration(i)*time.Minute))
	}

	rows, err := catalogue.ListEvaluations(ctx, tenant, 2)
	if err != nil {
		t.Fatalf("带 limit 上列：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit 2 交回 %d 行", len(rows))
	}

	if _, err := catalogue.ListEvaluations(ctx, tenant, 0); err == nil {
		t.Fatal("评价册 limit 0 未被拒")
	}
	if _, err := catalogue.ListEvaluations(ctx, tenant, -1); err == nil {
		t.Fatal("评价册 limit -1 未被拒")
	}
}
