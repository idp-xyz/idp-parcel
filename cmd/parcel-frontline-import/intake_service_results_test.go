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

// 本文件对真实 PostgreSQL 证前线导入在跨上下文读口读不动时的去向（ADR-0186，隔离合成 S）：用 DDL 把 PS 的一张表挪走，
// 不换服务结果读口、不改装配签名。服务结果读不动的行照常落地、明细写明读不动；身份核对读不动的行记未决、明细带真实原因，
// 挪回后重跑同一份文件就落地。

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

// Covers: ADR-0186「身份核对读不动：编排交回 error，不形成答案……前线导入记未决」——PS 外部标识关联变更册被挪走，`RECEIVED`
// 行记未决、明细带真实原因，另两行不经身份核对照常落地；挪回后重跑同一份文件，那一行落地，另两行答重放。
func TestFrontlineImportLeavesTheReceivedRowUndecidedWhenIdentityIsUnreadable(t *testing.T) {
	pool, importer := movableImporter(t, nil)
	restore := movePSTable(t, pool, "external_identifier_relation_change")

	results := importTemplate(t, importer, sampleTemplate(t))
	if got := results[0]; got.Disposition != rowUndecided || !strings.Contains(got.Detail, "external_identifier_relation_change") {
		t.Fatalf("第 1 行 → %s [%s]（%s），要未决且明细带真实原因", got.Outcome, got.Disposition, got.Detail)
	}
	assertDisposition(t, results, 1, rowLanded, "INTAKE_NOT_FORMED")
	if code := exitCodeFor(results); code != exitUndecided {
		t.Fatalf("退出码 = %d，要 %d", code, exitUndecided)
	}

	restore()
	rerun := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, rerun, 0, rowLanded, "PENDING_IDENTIFICATION")
	assertDisposition(t, rerun, 1, rowReplayed, "EXISTING_RESULT")
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
