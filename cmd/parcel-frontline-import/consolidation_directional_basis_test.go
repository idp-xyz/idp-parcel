package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证集运导入的方向依据记录（票 no-directional-basis/01 偏离标记半边，隔离合成 S）：已经
// 发生的移入照记、记录与事实同一行落、导入报告如实写缺少与读不动；PS 读口在 SQL 层出错时事实照落（判断在本行
// 写事务之前，见 application.DirectionalBasisJudge）。

const (
	sourceIDAddFirst  = "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0002"
	sourceIDAddSecond = "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0003"
)

type basisVertical struct {
	importer consolidationImporter
	facts    *nopostgres.ConsolidationFacts
	db       *bentopg.DB
	pool     *pgxpool.Pool
}

func newBasisVertical(t *testing.T) basisVertical {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	importer, err := buildConsolidationImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T18:00:00+08:00")})
	if err != nil {
		t.Fatalf("装配集运导入口：%v", err)
	}
	facts, err := nopostgres.NewConsolidationFacts(db)
	if err != nil {
		t.Fatalf("构造集运事实登记读口：%v", err)
	}
	return basisVertical{importer: importer, facts: facts, db: db, pool: pool}
}

// seedFormedIntake 在收寄册上落一行「收寄已形成」：样例里的 SYN-HU-0001 先由收寄批导入、识别到一件正式包裹。
func (vertical basisVertical) seedFormedIntake(t *testing.T, unit, parcel string) {
	t.Helper()
	tenant := synTenant(t)
	receivedAt := mustInstant(t, "2026-09-08T13:30:00+08:00")
	handlingUnit := mustRef(t, domain.NewHandlingUnitID, unit)
	node := mustRef(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB")
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    tenant,
		Unit:        handlingUnit,
		Node:        node,
		DeliveredBy: mustRef(t, domain.NewDeliveringPartyReference, "SYN-SHIPPER-01"),
		Evidence:    mustRef(t, domain.NewReceptionEvidenceReference, "FTI/INTAKE-1/SYN-CLERK-A/SYN-RECEIPT-0001"),
		Version:     mustRef(t, domain.NewIntakeResultVersion, "SYN-INTAKE-V1"),
		Association: mustRef(t, domain.NewParcelAssociationReference, parcel),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("构造收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      tenant,
		Unit:          handlingUnit,
		Node:          node,
		Kind:          domain.EstablishedByNodeIntake,
		Basis:         mustRef(t, domain.NewControlBasisReference, "SYN-INTAKE-V1"),
		EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(vertical.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	record := ports.ReceptionRecord{
		Key:            ports.ReceptionKey{TenantID: tenant, SourceID: "FTI/INTAKE-1/SYN-BATCH-20260908-AM/SYN-RECEIPT-0001"},
		ContentDigest:  "digest-syn-receipt-0001",
		Kind:           ports.RecordIntakeFormed,
		Intake:         intake,
		Control:        control,
		ServiceMarkers: []string{},
		RecordedAt:     receivedAt,
	}
	var saveErr error
	if err := vertical.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr = receptions.Save(txCtx, record)
		return saveErr
	}); err != nil {
		t.Fatalf("落收寄行：%v", err)
	}
}

func (vertical basisVertical) basisOf(t *testing.T, sourceID string) domain.DirectionalBasisRecord {
	t.Helper()
	record, found, err := vertical.facts.FindByKey(t.Context(), ports.ConsolidationFactKey{TenantID: synTenant(t), SourceID: sourceID})
	if err != nil || !found {
		t.Fatalf("读 %s：found=%v err=%v", sourceID, found, err)
	}
	return record.DirectionalBasis
}

func (vertical basisVertical) run(t *testing.T) ([]rowResult, string) {
	t.Helper()
	batch, err := decodeConsolidationTemplate(synTenant(t), sampleConsolidationTemplate(t))
	if err != nil {
		t.Fatalf("集运模板译装失败：%v", err)
	}
	var out bytes.Buffer
	results, tally := importConsolidation(t.Context(), batch, vertical.importer)
	writeImportReport(&out, batch.BatchRef, consolidationTemplateVersion, results)
	writeDirectionalBasisTally(&out, tally)
	return results, out.String()
}

func mustRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// TestImportedMembersCarryTheirDirectionalBasis 用样例模板：SYN-HU-0001 有收寄、识别到正式包裹，PS 上没有取消与
// 终局，身份与服务结果成立、路由依据未配置；SYN-HU-0002 在收寄册上查无此物，身份缺少·无收寄记录，另两格未判。两行
// 都照常落地，退出码不变；报告逐行写缺少依据，路由依据未配置只在汇总说一次；重跑交回册上那一份。
func TestImportedMembersCarryTheirDirectionalBasis(t *testing.T) {
	vertical := newBasisVertical(t)
	vertical.seedFormedIntake(t, "SYN-HU-0001", "SYN-DPCL-0001")

	results, report := vertical.run(t)
	assertDisposition(t, results, 1, rowLanded, "MEMBER_ADDED")
	assertDisposition(t, results, 2, rowLanded, "MEMBER_ADDED")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("退出码 = %d：缺依据不是拒收", code)
	}

	first := vertical.basisOf(t, sourceIDAddFirst)
	if first.Identity().Cell().Answer() != domain.BasisEstablished ||
		first.ServiceResult().Cell().Answer() != domain.BasisEstablished ||
		first.Routing().Cell().Answer() != domain.BasisNotConfigured {
		t.Fatalf("SYN-HU-0001 = %v", first.Cells())
	}
	if parcel, _ := first.Identity().Association(); parcel.String() != "SYN-DPCL-0001" {
		t.Fatalf("所依关联 = %q", parcel)
	}
	if !first.JudgedAt().Equal(mustInstant(t, "2026-09-08T18:00:00+08:00")) {
		t.Fatalf("判断时刻 = %s，要导入那一刻（不是装袋时）", first.JudgedAt())
	}
	second := vertical.basisOf(t, sourceIDAddSecond)
	if reasons := second.Identity().Cell().Reasons(); len(reasons) != 1 || reasons[0] != domain.ReasonNoIntakeRecord ||
		second.ServiceResult().Cell().Answer() != domain.BasisNotJudged || second.Routing().Cell().Answer() != domain.BasisNotJudged {
		t.Fatalf("SYN-HU-0002 = %v", second.Cells())
	}

	if strings.Contains(results[1].Detail, "缺少依据") || !strings.Contains(results[2].Detail, "缺少依据：身份·无收寄记录") {
		t.Fatalf("逐行说明：%q / %q", results[1].Detail, results[2].Detail)
	}
	if strings.Contains(results[1].Detail, "路由依据") {
		t.Fatalf("路由依据未配置逐行写了出来：%q", results[1].Detail)
	}
	if !strings.Contains(report, "移入 2 行，缺少依据 1 行（无收寄记录 1）") || !strings.Contains(report, "路由依据未配置 1 行") ||
		!strings.Contains(report, "身份不成立 1 行，服务结果与路由依据未判") {
		t.Fatalf("汇总行：\n%s", report)
	}

	replayed, _ := vertical.run(t)
	assertDisposition(t, replayed, 2, rowReplayed, "EXISTING_RESULT")
	if !strings.Contains(replayed[2].Detail, "缺少依据：身份·无收寄记录") ||
		!strings.Contains(replayed[2].Detail, "方向依据判于 "+mustInstant(t, "2026-09-08T18:00:00+08:00").UTC().Format(time.RFC3339)) {
		t.Fatalf("重放说明：%q", replayed[2].Detail)
	}
}

// TestAServiceResultReadFailureStillLandsTheMember 让 PS 的取消决定册在 SQL 层出错（表改名，查询报「relation does
// not exist」）。读要是落在本行写事务里，那一次读错会把整笔事务打进中止态、提交时回滚，本行成`NO_ANSWER_FORMED`；
// 判断挪到事务之前之后，事实照落，服务结果那一格记`读不动`。
func TestAServiceResultReadFailureStillLandsTheMember(t *testing.T) {
	vertical := newBasisVertical(t)
	vertical.seedFormedIntake(t, "SYN-HU-0001", "SYN-DPCL-0001")
	if _, err := vertical.pool.Exec(t.Context(),
		`ALTER TABLE parcel_shipment.parcel_cancellation RENAME TO parcel_cancellation_gone`); err != nil {
		t.Fatalf("让 PS 读口出错：%v", err)
	}

	results, report := vertical.run(t)
	assertDisposition(t, results, 1, rowLanded, "MEMBER_ADDED")
	first := vertical.basisOf(t, sourceIDAddFirst)
	if first.Identity().Cell().Answer() != domain.BasisEstablished || first.ServiceResult().Cell().Answer() != domain.BasisUnreadable {
		t.Fatalf("SYN-HU-0001 = %v", first.Cells())
	}
	if !strings.Contains(results[1].Detail, "读不动：服务结果") || !strings.Contains(report, "读不动 1 行") {
		t.Fatalf("说明：%q\n%s", results[1].Detail, report)
	}
}
