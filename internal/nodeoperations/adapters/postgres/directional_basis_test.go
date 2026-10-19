package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证方向依据记录的库面（票 no-directional-basis/01，迁移 0005）：随移入事实原样读回、
// 这一列出现之前的旧行读作`未记录`、绕过适配器写进来的坏形状由 CHECK 拦，以及按作业实物读收寄册。

var basisJudgedAt = time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)

func addMemberFactRecord(
	t *testing.T,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
	sourceID, member string,
	basis domain.DirectionalBasisRecord,
) ports.ConsolidationFactRecord {
	t.Helper()
	record := consolidationFactRecord(t, tenant, unit, sourceID)
	record.Action = domain.AddMemberAction
	record.Member = ref(t, domain.NewHandlingUnitID, member)
	record.DirectionalBasis = basis
	return record
}

func establishedWithCancelledAndFinal(t *testing.T) domain.DirectionalBasisRecord {
	t.Helper()
	identity, err := domain.JudgeIdentityBasis([]domain.IntakeIdentityObservation{{
		SourceID:    "src-intake-1",
		Version:     ref(t, domain.NewIntakeResultVersion, "intake/v1"),
		Association: ref(t, domain.NewParcelAssociationReference, "DPCL-1"),
		ReceivedAt:  receivedAt,
	}})
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	service, err := domain.JudgeServiceResultBasis("CANCEL-1", "RETURNED", "FINAL-2")
	if err != nil {
		t.Fatalf("service result: %v", err)
	}
	record, err := domain.RecordDirectionalBasis(identity, service, domain.NotConfiguredRoutingBasis(), basisJudgedAt)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	return record
}

// TestADirectionalBasisRecordRoundTripsWithTheAddMemberFact 证记录随移入事实同一行落、经领域重建门原样读回：
// 三格取值与原因、身份所依的收寄、服务结果引用、判断时刻；身份不成立的一份另两格读回`未判`；不带记录写下的移入
// 行（这一列出现之前的样子）读回零值，读作`未记录`。
func TestADirectionalBasisRecordRoundTripsWithTheAddMemberFact(t *testing.T) {
	facts, units, transactor, _ := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	full := addMemberFactRecord(t, tenant, unit, "src-add-full", "hu-1", establishedWithCancelledAndFinal(t))
	saveConsolidationFact(t, transactor, ctx, facts, full)
	read, found, err := facts.FindByKey(ctx, full.Key)
	if err != nil || !found {
		t.Fatalf("find: found=%v err=%v", found, err)
	}
	basis := read.DirectionalBasis
	if !basis.Recorded() || !basis.JudgedAt().Equal(basisJudgedAt) {
		t.Fatalf("judged at = %v recorded=%v", basis.JudgedAt(), basis.Recorded())
	}
	if parcel, ok := basis.Identity().Association(); !ok || parcel.String() != "DPCL-1" ||
		basis.Identity().IntakeSourceID() != "src-intake-1" || basis.Identity().IntakeVersion().String() != "intake/v1" {
		t.Fatalf("identity = %+v", basis.Identity())
	}
	service := basis.ServiceResult()
	if service.Cell().Answer() != domain.BasisMissing || len(service.Cell().Reasons()) != 2 || service.Cancellation() != "CANCEL-1" {
		t.Fatalf("service result = %+v", service)
	}
	if kind, version := service.Final(); kind != "RETURNED" || version != "FINAL-2" {
		t.Fatalf("final = %q %q", kind, version)
	}
	if basis.Routing().Cell().Answer() != domain.BasisNotConfigured {
		t.Fatalf("routing = %s", basis.Routing().Cell().Answer())
	}

	identity, err := domain.JudgeIdentityBasis(nil)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	withoutIdentity, err := domain.RecordDirectionalBasisWithoutIdentity(identity, basisJudgedAt)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	missing := addMemberFactRecord(t, tenant, unit, "src-add-missing", "hu-2", withoutIdentity)
	saveConsolidationFact(t, transactor, ctx, facts, missing)
	read, _, err = facts.FindByKey(ctx, missing.Key)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got := read.DirectionalBasis.Identity().Cell().Reasons(); len(got) != 1 || got[0] != domain.ReasonNoIntakeRecord {
		t.Fatalf("identity reasons = %v", got)
	}
	if read.DirectionalBasis.ServiceResult().Cell().Answer() != domain.BasisNotJudged ||
		read.DirectionalBasis.Routing().Cell().Answer() != domain.BasisNotJudged {
		t.Fatalf("cells = %v", read.DirectionalBasis.Cells())
	}

	legacy := addMemberFactRecord(t, tenant, unit, "src-add-legacy", "hu-3", domain.DirectionalBasisRecord{})
	saveConsolidationFact(t, transactor, ctx, facts, legacy)
	read, _, err = facts.FindByKey(ctx, legacy.Key)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if read.DirectionalBasis.Recorded() {
		t.Fatalf("a fact written without a record read back as recorded: %v", read.DirectionalBasis.Cells())
	}
}

// TestDirectionalBasisColumnRejectsImpossibleRows 守迁移 0005 加在这一列上的 CHECK：绕过适配器直接写库的一行——迁移脚本、
// 运维改数、日后另写的适配器——领域构造器一次都不会经过，这是它们唯一过的门。
func TestDirectionalBasisColumnRejectsImpossibleRows(t *testing.T) {
	_, units, transactor, pool := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	insert := func(sourceID, action string, member any, basis string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO node_operations.consolidation_fact
				(tenant_id, source_id, content_digest, action, unit_id, member_id, seal_ref,
				 performed_by, evidence, occurred_at, recorded_at, directional_basis)
			 VALUES ('tenant-a', $1, 'digest-x', $2, 'bag-1', $3, NULL,
			         'packer-1', 'WORK-EVIDENCE/x', now(), now(), $4::jsonb)`,
			sourceID, action, member, basis)
		return err
	}
	const unjudged = `{"answer":"NOT_JUDGED"}`
	good := `{"judgedAt":"2026-10-16T09:30:00Z","identity":{"answer":"MISSING","reasons":["NO_INTAKE_RECORD"]},` +
		`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`

	rejected := map[string]struct {
		action string
		member any
		basis  string
	}{
		"非移入行带记录": {"OPEN_UNIT", nil, good},
		"缺判断时刻": {"ADD_MEMBER", "hu-1", `{"identity":{"answer":"MISSING","reasons":["NO_INTAKE_RECORD"]},` +
			`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`},
		"缺一格": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z",` +
			`"identity":{"answer":"MISSING","reasons":["NO_INTAKE_RECORD"]},"routing":` + unjudged + `}`},
		"取值在词表外": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z","identity":{"answer":"PROBABLY"},` +
			`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`},
		"缺少却没有原因": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z","identity":{"answer":"MISSING"},` +
			`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`},
		"成立却带原因": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z",` +
			`"identity":{"answer":"ESTABLISHED","reasons":["PENDING_IDENTIFICATION"]},` +
			`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`},
		"原因不属这一格": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z",` +
			`"identity":{"answer":"MISSING","reasons":["CANCELLED"]},` +
			`"serviceResult":` + unjudged + `,"routing":` + unjudged + `}`},
		"路由依据带原因": {"ADD_MEMBER", "hu-1", `{"judgedAt":"2026-10-16T09:30:00Z",` +
			`"identity":{"answer":"MISSING","reasons":["NO_INTAKE_RECORD"]},"serviceResult":` + unjudged +
			`,"routing":{"answer":"MISSING","reasons":["NO_CURRENT_ROUTE"]}}`},
	}
	for name, row := range rejected {
		err := insert("bad-"+name, row.action, row.member, row.basis)
		want := "consolidation_fact_directional_basis_shaped"
		if row.action != "ADD_MEMBER" {
			want = "consolidation_fact_directional_basis_add_member_only"
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != want {
			t.Errorf("一行「%s」该被 %s 拦下，实得：%v", name, want, err)
		}
	}
	if err := insert("good-add", "ADD_MEMBER", "hu-1", good); err != nil {
		t.Errorf("合法的一行被拒，上面的拒绝因此说明不了是这一列的 CHECK 在起作用：%v", err)
	}
}

// TestIntakeIdentitiesReadsTheUnitsIntakeRows 证按作业实物读收寄册：收寄已形成那一行带关联、待识别那一行不带关联
// 而带身份冲突、未形成的行没有收寄也就不出现；查无此物与别的租户都交回空，不是错误。
func TestIntakeIdentitiesReadsTheUnitsIntakeRows(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	view, err := adapter.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	ctx := t.Context()
	inTx(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{
			formedRecord(t, "tenant-a", "src-formed"),
			pendingRecord(t, "tenant-a", "src-pending"),
			notFormedRecord(t, "tenant-a", "src-refused"),
		} {
			if _, err := receptions.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	formed, err := view.FindIntakeIdentities(ctx, tenant, ref(t, domain.NewHandlingUnitID, "unit-1"))
	if err != nil {
		t.Fatalf("find unit-1: %v", err)
	}
	if len(formed) != 1 || formed[0].SourceID != "src-formed" || formed[0].Association.String() != "parcel-1/link-v1" ||
		formed[0].Version.String() != "intake/v1" || formed[0].IdentityConflict || !formed[0].ReceivedAt.Equal(receivedAt) {
		t.Fatalf("unit-1 observations = %+v", formed)
	}

	pending, err := view.FindIntakeIdentities(ctx, tenant, ref(t, domain.NewHandlingUnitID, "unit-2"))
	if err != nil {
		t.Fatalf("find unit-2: %v", err)
	}
	if len(pending) != 1 || pending[0].Association.String() != "" || !pending[0].IdentityConflict {
		t.Fatalf("unit-2 observations = %+v", pending)
	}

	for _, probe := range []struct{ tenant, unit string }{{"tenant-a", "unit-9"}, {"tenant-b", "unit-1"}} {
		none, err := view.FindIntakeIdentities(ctx, ref(t, domain.NewTenantID, probe.tenant), ref(t, domain.NewHandlingUnitID, probe.unit))
		if err != nil || none == nil || len(none) != 0 {
			t.Fatalf("%s/%s = %+v err=%v, want an empty slice", probe.tenant, probe.unit, none, err)
		}
	}
}

// Covers: ADR-0210 决定五的读口面——按作业实物读收寄册时连前版一并读回：追加的一版所指名的前版逐字一致，没有前版的
// 行（首判，或前版机制之前落下的行）读回为缺席。读回的这一组交给领域，链头只剩那一版冲突版，落成缺少·{待识别, 身份
// 冲突}。夹具全为合成（S 级）。
func TestIntakeIdentitiesReadsThePredecessorOfEachVersion(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	view, err := adapter.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}

	original := formedRecord(t, "SYN-TENANT-NRI05", "SYN-DELIVERY-NRI05-A")
	original.JudgmentVersion = original.Intake.Version()
	conflict := pendingRecord(t, "SYN-TENANT-NRI05", "SYN-DELIVERY-NRI05-B")
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    original.Key.TenantID,
		Unit:        original.Intake.Unit(),
		Node:        original.Intake.Node(),
		DeliveredBy: original.Intake.DeliveredBy(),
		Evidence:    original.Intake.Evidence(),
		Version:     ref(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI05-CONFLICT"),
		ReceivedAt:  original.Intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatalf("构造冲突版收寄：%v", err)
	}
	conflict.Intake = intake
	conflict.Candidates = []domain.ParcelAssociationReference{ref(t, domain.NewParcelAssociationReference, "parcel-1/link-v1")}
	conflict.JudgmentVersion = intake.Version()
	conflict.Predecessor = original.JudgmentVersion
	ctx := t.Context()
	inTx(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{original, conflict} {
			if _, err := receptions.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	observations, err := view.FindIntakeIdentities(ctx, original.Key.TenantID, original.Intake.Unit())
	if err != nil {
		t.Fatalf("find unit-1: %v", err)
	}
	if len(observations) != 2 {
		t.Fatalf("unit-1 observations = %+v, want 原判断与冲突版两行", observations)
	}
	if first := observations[0]; first.SourceID != "SYN-DELIVERY-NRI05-A" || first.Predecessor.String() != "" {
		t.Fatalf("原判断 = %+v, want 前版缺席", first)
	}
	if second := observations[1]; second.SourceID != "SYN-DELIVERY-NRI05-B" ||
		second.Version.String() != "SYN-INTAKEV-NRI05-CONFLICT" || second.Predecessor.String() != "intake/v1" {
		t.Fatalf("冲突版 = %+v, want 前版逐字是原判断的版本 intake/v1", second)
	}
	basis, err := domain.JudgeIdentityBasis(observations)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	reasons := basis.Cell().Reasons()
	if len(reasons) != 2 || reasons[0] != domain.ReasonPendingIdentification || reasons[1] != domain.ReasonIdentityConflict {
		t.Fatalf("身份一格 = %s %v, want 缺少·{待识别, 身份冲突}", basis.Cell().Answer(), reasons)
	}
}

// TestConsolidationMemberCatalogueListsAddAndRemoveFacts 证「集运成员关系」册只列移入与移出两种来源事实，按记录时刻
// 倒序；移入行的方向依据记录原样读回，没有记录的移入行与移出行都交回零值。
func TestConsolidationMemberCatalogueListsAddAndRemoveFacts(t *testing.T) {
	facts, units, transactor, pool := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	saveConsolidationFact(t, transactor, ctx, facts, consolidationFactRecord(t, tenant, unit, "src-open"))
	judged := addMemberFactRecord(t, tenant, unit, "src-add-judged", "hu-1", establishedWithCancelledAndFinal(t))
	judged.RecordedAt = consolidationAt.Add(2 * time.Minute)
	saveConsolidationFact(t, transactor, ctx, facts, judged)
	legacy := addMemberFactRecord(t, tenant, unit, "src-add-legacy", "hu-2", domain.DirectionalBasisRecord{})
	legacy.RecordedAt = consolidationAt.Add(3 * time.Minute)
	saveConsolidationFact(t, transactor, ctx, facts, legacy)
	removed := addMemberFactRecord(t, tenant, unit, "src-remove", "hu-2", domain.DirectionalBasisRecord{})
	removed.Action = domain.RemoveMemberAction
	removed.RecordedAt = consolidationAt.Add(4 * time.Minute)
	saveConsolidationFact(t, transactor, ctx, facts, removed)

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	rows, err := catalogue.ListConsolidationMembers(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 || rows[0].SourceID != "src-remove" || rows[1].SourceID != "src-add-legacy" || rows[2].SourceID != "src-add-judged" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Action != "REMOVE_MEMBER" || rows[0].Member != "hu-2" || rows[0].DirectionalBasis.Recorded() {
		t.Fatalf("remove row = %+v", rows[0])
	}
	if rows[1].DirectionalBasis.Recorded() {
		t.Fatalf("legacy add row read back as recorded")
	}
	if !rows[2].DirectionalBasis.Recorded() || rows[2].DirectionalBasis.ServiceResult().Cancellation() != "CANCEL-1" {
		t.Fatalf("judged add row basis = %v", rows[2].DirectionalBasis.Cells())
	}
	other, err := catalogue.ListConsolidationMembers(ctx, ref(t, domain.NewTenantID, "tenant-b"), 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("tenant-b = %+v err=%v", other, err)
	}
}
