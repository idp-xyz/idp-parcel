package main

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证前线导入在跨上下文读口读不动时的去向（ADR-0186、ADR-0199 决定五，隔离合成 S）：用 DDL 把 PS 的
// 一张表挪走，不换读口、不改装配签名。服务结果读不动的行照常落地、明细写明读不动；身份核对读不动的行照常落地成未核对的待识别、
// 明细写明候选缺席不代表查无此号，挪回后重跑同一份文件答重放。

func movableImporter(t *testing.T, identity ports.ParcelIdentityView) (*pgxpool.Pool, intakeImporter) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	if identity == nil {
		identity, err = parcelIdentityViewOf(db)
		if err != nil {
			t.Fatalf("装配身份核对：%v", err)
		}
	}
	importer, err := buildIntakeImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}, identity)
	if err != nil {
		t.Fatalf("装配导入口：%v", err)
	}
	return pool, importer
}

func movePSTable(t *testing.T, pool *pgxpool.Pool, table string) (restore func()) {
	t.Helper()
	moved := table + "_moved_oc29"
	if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.`+table+` RENAME TO `+moved); err != nil {
		t.Fatalf("挪走 %s：%v", table, err)
	}
	return func() {
		t.Helper()
		if _, err := pool.Exec(t.Context(), `ALTER TABLE parcel_shipment.`+moved+` RENAME TO `+table); err != nil {
			t.Fatalf("挪回 %s：%v", table, err)
		}
	}
}

// Covers: ADR-0186「身份唯一，但服务结果读不动：照常`节点收寄已形成`」的前线导入面——PS 取消决定册被挪走，身份唯一的
// `RECEIVED` 行首导即已落地、退出码 0，明细写明服务结果读不动；身份核对由能解析的替身答，服务结果读口是生产装配的。
func TestFrontlineImportLandsTheReceivedRowWhenServiceResultsAreUnreadable(t *testing.T) {
	association, err := domain.NewParcelAssociationReference("SYN-PARCEL-0001")
	if err != nil {
		t.Fatalf("构造包裹关联：%v", err)
	}
	pool, importer := movableImporter(t, resolvingParcelIdentityView{association: association})
	movePSTable(t, pool, "parcel_cancellation")

	results := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, results, 0, rowLanded, "INTAKE_FORMED")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("退出码 = %d，要 %d（%+v）", code, exitLanded, results)
	}
	if !strings.Contains(results[0].Detail, "服务结果读不动：标记缺席不代表没有取消或终局") {
		t.Fatalf("明细没写明读不动：%s", results[0].Detail)
	}
}

// Covers: `AT-NO-051` 与 ADR-0199 决定五的前线导入面（「前线导入明细写『身份未核对：候选缺席不代表查无此号』，这一行照常记成功、
// 不再记未决」）——PS 外部标识关联变更册被挪走，`RECEIVED` 行落地成未核对的待识别、明细写明候选缺席不代表查无此号，另两行照常
// 落地，退出码 0；同一笔自投门铃一封。挪回后重跑同一份文件，那一行答重放、明细仍写未核对（续上靠门铃重核，不靠重跑）。PS 读得
// 动时同一个号查无此号，明细写已核对，两种待识别在明细上分得开。
func TestFrontlineImportLandsTheReceivedRowUncheckedWhenIdentityIsUnreadable(t *testing.T) {
	pool, importer := movableImporter(t, nil)
	restore := movePSTable(t, pool, "external_identifier_relation_change")

	results := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, results, 0, rowLanded, "PENDING_IDENTIFICATION")
	if !strings.Contains(results[0].Detail, "身份未核对：候选缺席不代表查无此号") {
		t.Fatalf("明细没写明未核对：%s", results[0].Detail)
	}
	assertDisposition(t, results, 1, rowLanded, "INTAKE_NOT_FORMED")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("退出码 = %d，要 %d（%+v）", code, exitLanded, results)
	}
	var doorbells int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		"node-operations.unchecked-pending-identity.recorded").Scan(&doorbells); err != nil || doorbells != 1 {
		t.Fatalf("门铃 %d 封 err %v, want 1", doorbells, err)
	}

	restore()
	rerun := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, rerun, 0, rowReplayed, "EXISTING_RESULT")
	if !strings.Contains(rerun[0].Detail, "身份未核对") {
		t.Fatalf("重放明细没写明未核对：%s", rerun[0].Detail)
	}
	assertDisposition(t, rerun, 1, rowReplayed, "EXISTING_RESULT")

	_, readable := movableImporter(t, nil)
	checked := importTemplate(t, readable, sampleTemplate(t))
	assertDisposition(t, checked, 0, rowLanded, "PENDING_IDENTIFICATION")
	if !strings.Contains(checked[0].Detail, "身份已核对") || strings.Contains(checked[0].Detail, "未核对") {
		t.Fatalf("查无此号的明细 = %s, want 写已核对、不写未核对", checked[0].Detail)
	}
}

// Covers: ADR-0186「输入缺陷两支，交回被拒类错误……前线导入记被拒」——模板译装挡在前面，这一支只在命令绕过译装时出现；
// 缺发生时间的一行记被拒，不记未决：重跑同一份文件不会变好。
func TestFrontlineImportRejectsACommandTheOrchestrationRefuses(t *testing.T) {
	_, importer := movableImporter(t, nil)
	batch, err := decodeIntakeTemplate(synTenant(t), sampleTemplate(t))
	if err != nil {
		t.Fatalf("模板译装失败：%v", err)
	}
	row := batch.Rows[1]
	row.Command.OccurredAt = time.Time{}

	results := importIntake(t.Context(), intakeBatch{BatchRef: batch.BatchRef, Rows: []intakeRow{row}}, importer)
	if got := results[0]; got.Disposition != rowRejected {
		t.Fatalf("→ %s [%s]（%s），要被拒", got.Outcome, got.Disposition, got.Detail)
	}
}
