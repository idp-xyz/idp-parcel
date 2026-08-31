package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证治理登记册三册读面（票 admin-skeleton-closure-batch/02，
// 键形依 ADR-0083）：语句不带租户条件（表没有那一列，Decision 一）、检索列面照登
// 转写、盘点 jsonb 不透出、开放区间如实缺席上界、空册答空列表、limit 生效且非正拒。
// 夹具经写侧仓储铺设（S 级合成登记，SYN- 前缀）——治理册有受控写路（登记 CLI 的
// store 半边），读面测试直接借用，不另开 SQL 插行。

var registryReadAt = time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

type registryReadFixture struct {
	registers   *adapter.GovernanceRegisters
	intervals   *adapter.AuthorityIntervals
	suspensions *adapter.Suspensions
	resumptions *adapter.Resumptions
	transactor  bentoapp.Transactor
}

func newRegistryReadFixture(t *testing.T) *registryReadFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registers, err := adapter.NewGovernanceRegisters(db)
	if err != nil {
		t.Fatalf("构造登记册读面：%v", err)
	}
	intervals, err := adapter.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatalf("构造区间库：%v", err)
	}
	suspensions, err := adapter.NewSuspensions(db)
	if err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	resumptions, err := adapter.NewResumptions(db)
	if err != nil {
		t.Fatalf("构造恢复库：%v", err)
	}
	return &registryReadFixture{
		registers:   registers,
		intervals:   intervals,
		suspensions: suspensions,
		resumptions: resumptions,
		transactor:  db.Transactor(),
	}
}

func (fixture *registryReadFixture) write(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// registrySuspension 与 incidentSuspension 同形，但暂停标识、范围与发生时刻可参数化
// ——排序断言需要不同时刻的行。
func registrySuspension(t *testing.T, id, scope string, occurredAt time.Time) domain.SuspensionDecision {
	t.Helper()
	decision, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            govRef(t, domain.NewSuspensionID, id),
		TriggerSource: "SYN-TRIGGER/shadow-diff-alarm",
		Basis:         "SYN-BASIS/stage-review-no-go",
		Evidence:      "SYN-EVIDENCE/incident-260803",
		Scope:         govRef(t, domain.NewScopeVersionReference, scope),
		ExecutedBy:    "SYN-GOV-OPERATOR",
		OccurredAt:    occurredAt,
		EffectiveAt:   occurredAt.Add(time.Hour),
		InTransitNote: "SYN-NOTE/in-transit objects stay with current authority",
	})
	if err != nil {
		t.Fatalf("形成暂停：%v", err)
	}
	return decision
}

// Covers: 权威区间册照列转写——四维身份与生效区间照登透出，开放区间的上界如实
// 缺席，代理键不透出；排序按登记时间倒序稳定；空册答空；limit 非正拒。
func TestAuthorityIntervalRegistryTranscribesTheColumnFace(t *testing.T) {
	fixture := newRegistryReadFixture(t)
	ctx := t.Context()

	empty, err := fixture.registers.ListAuthorityIntervals(ctx, 10)
	if err != nil {
		t.Fatalf("空册上列：%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空册答了 %d 行，want 0", len(empty))
	}

	closed := domain.AuthorityInterval{
		ObjectScope: "SYN-SCOPE/routing-pilot",
		Capability:  "SYN-CAP/route-planning",
		FactKind:    "SYN-FACT/route-plan",
		Authority:   "SYN-AUTH/legacy-engine",
		From:        registryReadAt.Add(-30 * 24 * time.Hour),
		To:          registryReadAt,
	}
	handover := domain.AuthorityInterval{
		ObjectScope: "SYN-SCOPE/routing-pilot",
		Capability:  "SYN-CAP/route-planning",
		FactKind:    "SYN-FACT/route-plan",
		Authority:   "SYN-AUTH/pilot-engine",
		From:        registryReadAt,
	}
	for _, interval := range []domain.AuthorityInterval{closed, handover} {
		fixture.write(t, ctx, func(txCtx context.Context) error {
			return fixture.intervals.Append(txCtx, interval)
		})
	}

	rows, err := fixture.registers.ListAuthorityIntervals(ctx, 10)
	if err != nil {
		t.Fatalf("上列区间：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，want 2", len(rows))
	}
	// 登记时间倒序：后追加的交接区间在前。
	open, ended := rows[0], rows[1]
	if open.Authority != "SYN-AUTH/pilot-engine" || ended.Authority != "SYN-AUTH/legacy-engine" {
		t.Fatalf("排序变形：%q, %q", open.Authority, ended.Authority)
	}
	if open.ObjectScope != "SYN-SCOPE/routing-pilot" || open.Capability != "SYN-CAP/route-planning" ||
		open.FactKind != "SYN-FACT/route-plan" || !open.FromAt.Equal(registryReadAt) {
		t.Fatalf("四维身份或起点变形：%+v", open)
	}
	if open.HasToAt {
		t.Fatalf("开放区间长出了上界：%+v", open)
	}
	if !ended.HasToAt || !ended.ToAt.Equal(registryReadAt) {
		t.Fatalf("已闭区间的上界没照登透出：%+v", ended)
	}
	if open.InsertedAt.IsZero() {
		t.Fatal("登记时间没透出")
	}

	if _, err := fixture.registers.ListAuthorityIntervals(ctx, 0); err == nil {
		t.Fatal("区间册 limit 0 未被拒")
	}
}

// Covers: 暂停与恢复两册照列转写——暂停九件照登；恢复的盘点 jsonb 不上列、盘点
// 时刻照登透出；未被恢复的暂停不出现在恢复册（两册各自成册，替代关系由页面对照）；
// limit 生效且非正拒。
func TestSuspensionAndResumptionRegistriesTranscribeTheColumnFace(t *testing.T) {
	fixture := newRegistryReadFixture(t)
	ctx := t.Context()

	first := registrySuspension(t, "SYN-GOV-SUS-0001", "SYN-PILOT-SCOPE@v3", registryReadAt)
	second := registrySuspension(t, "SYN-GOV-SUS-0002", "SYN-PILOT-SCOPE@v4", registryReadAt.Add(time.Hour))
	for _, decision := range []domain.SuspensionDecision{first, second} {
		fixture.write(t, ctx, func(txCtx context.Context) error {
			_, err := fixture.suspensions.Save(txCtx, decision)
			return err
		})
	}

	resumption, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       govRef(t, domain.NewSuspensionID, "SYN-GOV-SUS-0001"),
		ReleaseEvidence:  "SYN-EVIDENCE/release-260804",
		ConsistencyCheck: "SYN-CHECK/ledger-consistent",
		Inventory:        incidentInventory(t, "SYN-OBJ-0001"),
		DecidedBy:        "SYN-GOV-OPERATOR",
		DecidedAt:        registryReadAt.Add(26 * time.Hour),
		EffectiveAt:      registryReadAt.Add(27 * time.Hour),
	})
	if err != nil {
		t.Fatalf("形成恢复：%v", err)
	}
	fixture.write(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.resumptions.Save(txCtx, resumption)
		return err
	})

	suspensions, err := fixture.registers.ListSuspensions(ctx, 10)
	if err != nil {
		t.Fatalf("上列暂停：%v", err)
	}
	if len(suspensions) != 2 {
		t.Fatalf("上列 %d 行，want 2", len(suspensions))
	}
	// 发生时刻倒序：后发生的 0002 在前。
	if suspensions[0].SuspensionID != "SYN-GOV-SUS-0002" || suspensions[1].SuspensionID != "SYN-GOV-SUS-0001" {
		t.Fatalf("排序变形：%q, %q", suspensions[0].SuspensionID, suspensions[1].SuspensionID)
	}
	suspended := suspensions[1]
	if suspended.TriggerSource != "SYN-TRIGGER/shadow-diff-alarm" ||
		suspended.Basis != "SYN-BASIS/stage-review-no-go" ||
		suspended.Evidence != "SYN-EVIDENCE/incident-260803" ||
		suspended.Scope != "SYN-PILOT-SCOPE@v3" ||
		suspended.ExecutedBy != "SYN-GOV-OPERATOR" ||
		suspended.InTransitNote != "SYN-NOTE/in-transit objects stay with current authority" {
		t.Fatalf("暂停列面变形：%+v", suspended)
	}
	if !suspended.OccurredAt.Equal(registryReadAt) || !suspended.EffectiveAt.Equal(registryReadAt.Add(time.Hour)) {
		t.Fatalf("暂停两时刻变形：%+v", suspended)
	}

	resumptions, err := fixture.registers.ListResumptions(ctx, 10)
	if err != nil {
		t.Fatalf("上列恢复：%v", err)
	}
	if len(resumptions) != 1 {
		t.Fatalf("上列 %d 行，want 1（未被恢复的暂停不出现在恢复册）", len(resumptions))
	}
	resumed := resumptions[0]
	if resumed.SuspensionID != "SYN-GOV-SUS-0001" ||
		resumed.ReleaseEvidence != "SYN-EVIDENCE/release-260804" ||
		resumed.ConsistencyCheck != "SYN-CHECK/ledger-consistent" ||
		resumed.DecidedBy != "SYN-GOV-OPERATOR" {
		t.Fatalf("恢复列面变形：%+v", resumed)
	}
	if resumed.InventoryTakenAt.IsZero() || !resumed.DecidedAt.Equal(registryReadAt.Add(26*time.Hour)) {
		t.Fatalf("恢复时刻列变形：%+v", resumed)
	}

	limited, err := fixture.registers.ListSuspensions(ctx, 1)
	if err != nil {
		t.Fatalf("带 limit 上列：%v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit 1 交回 %d 行", len(limited))
	}
	if _, err := fixture.registers.ListSuspensions(ctx, -1); err == nil {
		t.Fatal("暂停册 limit -1 未被拒")
	}
	if _, err := fixture.registers.ListResumptions(ctx, 0); err == nil {
		t.Fatal("恢复册 limit 0 未被拒")
	}
}
