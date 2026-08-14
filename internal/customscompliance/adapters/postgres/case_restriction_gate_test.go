package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证案件、限制与门禁三库的行为：案件四维身份键与包裹/
// 角色 jsonb 原样往返、限制的建立代数与解除状态推进分开、门禁同指纹不出第二版、
// 封闭集合与解除形状入库内 CHECK（IS NULL 显式分支）、租户隔离由 SQL 条件承担。
// 断言一律在事务闭包外（Goexit 会挂死连接）。

var crgBaseAt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

type crgFixture struct {
	cases        *adapter.CustomsCases
	restrictions *adapter.Restrictions
	gates        *adapter.GateVerifications
	transactor   bentoapp.Transactor
	pool         *pgxpool.Pool
}

func newCRGFixture(t *testing.T) *crgFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cases, err := adapter.NewCustomsCases(db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	restrictions, err := adapter.NewRestrictions(db)
	if err != nil {
		t.Fatalf("构造限制库：%v", err)
	}
	gates, err := adapter.NewGateVerifications(db)
	if err != nil {
		t.Fatalf("构造门禁库：%v", err)
	}
	return &crgFixture{
		cases: cases, restrictions: restrictions, gates: gates,
		transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *crgFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func crgValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func caseKey(t *testing.T, tenant string) ports.CustomsCaseKey {
	t.Helper()
	return ports.CustomsCaseKey{
		TenantID:     crgValue(t, domain.NewTenantID, tenant),
		Jurisdiction: crgValue(t, domain.NewRegulatoryJurisdictionReference, "jurisdiction/US"),
		Direction:    domain.ImportManifest,
		Procedure:    crgValue(t, domain.NewCustomsProcedureReference, "IMPORT_STANDARD"),
		Obligation:   crgValue(t, domain.NewObligationScopeReference, "obligation/full"),
	}
}

func establishedCase(t *testing.T, key ports.CustomsCaseKey, caseID string, roles []domain.CaseRoleSnapshot) domain.CustomsCase {
	t.Helper()
	customsCase, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:           crgValue(t, domain.NewCustomsCaseID, caseID),
		Jurisdiction: key.Jurisdiction,
		Direction:    key.Direction,
		Procedure:    key.Procedure,
		Obligation:   key.Obligation,
		Parcels: []domain.CaseParcelAssociation{
			{Parcel: "parcel-1", Customer: "customer-1", SourceRef: "source/submission-1"},
			{Parcel: "parcel-2", Customer: "customer-1", SourceRef: "source/submission-2"},
		},
		Roles:         roles,
		EstablishedAt: crgBaseAt,
	})
	if err != nil {
		t.Fatalf("建立案件：%v", err)
	}
	return customsCase
}

// TestCaseIsReadBackUnchanged 证案件往返：四维身份键定位、包裹关联与角色快照 jsonb
// 原样读回（角色空清单如实空白），读回经 EstablishCustomsCase 整门重验。
func TestCaseIsReadBackUnchanged(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	key := caseKey(t, "tenant-a")
	withRoles := establishedCase(t, key, "case-1", []domain.CaseRoleSnapshot{
		{Role: "IMPORTER_OF_RECORD", Party: "party-1", Authority: "authority/mandate-1"},
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, key, withRoles)
		return err
	})

	found, exists, err := fixture.cases.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("读回案件：%v exists=%v", err, exists)
	}
	if found.ID().String() != "case-1" || !found.EstablishedAt().Equal(crgBaseAt) {
		t.Fatalf("案件没原样读回：%+v", found)
	}
	parcels := found.Parcels()
	if len(parcels) != 2 || parcels[0].Parcel != "parcel-1" || parcels[1].SourceRef != "source/submission-2" {
		t.Fatalf("包裹关联没原样读回：%+v", parcels)
	}
	roles := found.Roles()
	if len(roles) != 1 || roles[0].Role != "IMPORTER_OF_RECORD" || roles[0].Authority != "authority/mandate-1" {
		t.Fatalf("角色快照没原样读回：%+v", roles)
	}

	// 角色空清单是如实空白，不是坏行。
	emptyRolesKey := caseKey(t, "tenant-b")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, emptyRolesKey, establishedCase(t, emptyRolesKey, "case-2", nil))
		return err
	})
	bare, exists, err := fixture.cases.FindByKey(ctx, emptyRolesKey)
	if err != nil || !exists {
		t.Fatalf("读回空角色案件：%v exists=%v", err, exists)
	}
	if len(bare.Roles()) != 0 {
		t.Fatalf("空角色清单读回凭空长出角色：%+v", bare.Roles())
	}
}

// TestSecondCaseOnSameScopeGetsAlreadyRecorded 证同一法律行为一案：同四维第二个
// 案件答`已有记录`且原案件不被顶替，事务保持可用；键与案件分岔的写入在门口拦。
func TestSecondCaseOnSameScopeGetsAlreadyRecorded(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	key := caseKey(t, "tenant-a")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, key, establishedCase(t, key, "case-1", nil))
		return err
	})

	var outcome ports.CustomsCaseSaveOutcome
	var winner domain.CustomsCase
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.cases.Save(txCtx, key, establishedCase(t, key, "case-2", nil))
		if err != nil {
			return err
		}
		outcome = saved
		found, _, err := fixture.cases.FindByKey(txCtx, key)
		winner = found
		return err
	})
	if outcome != ports.CustomsCaseAlreadyRecorded {
		t.Fatalf("重写 outcome = %d, 想要 AlreadyRecorded", outcome)
	}
	if winner.ID().String() != "case-1" {
		t.Fatalf("原案件被顶替成 %s", winner.ID())
	}

	// 跨租户同四维各自成案。
	foreign := caseKey(t, "tenant-b")
	var foreignOutcome ports.CustomsCaseSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.cases.Save(txCtx, foreign, establishedCase(t, foreign, "case-3", nil))
		foreignOutcome = saved
		return err
	})
	if foreignOutcome != ports.CustomsCaseSaved {
		t.Fatalf("另一租户同四维 outcome = %d, 想要 Saved", foreignOutcome)
	}

	// 键与案件分岔：键说进口、案件是出口——不是代数答案，是调用方立不住的写入。
	mismatched := key
	mismatched.Direction = domain.ExportManifest
	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, mismatched, establishedCase(t, key, "case-4", nil))
		return err
	})
	if err == nil {
		t.Fatalf("键与案件分岔的写入被接受了")
	}
}

func establishedRestriction(t *testing.T, id, scope string, constrains []domain.GuardedAction) domain.RegulatoryRestriction {
	t.Helper()
	restriction, err := domain.EstablishRestriction(domain.RegulatoryRestrictionSpec{
		ID:          crgValue(t, domain.NewRestrictionID, id),
		Decision:    crgValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Scope:       crgValue(t, domain.NewDecisionScopeReference, scope),
		Constrains:  constrains,
		EffectiveAt: crgBaseAt,
	})
	if err != nil {
		t.Fatalf("建立限制：%v", err)
	}
	return restriction
}

// TestRestrictionEstablishAndReleaseAreSeparateSteps 证建立与解除分开：建立走代数
// （同标识第二份答`已有记录`），解除是同键状态推进（Update 后 Current 翻面、依据与
// 时刻原样读回）；ListByScope 盘出该范围全部限制供准入判断，已解除的由领域自己跳过。
func TestRestrictionEstablishAndReleaseAreSeparateSteps(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	holding := establishedRestriction(t, "restriction-1", "scope/parcel-1",
		[]domain.GuardedAction{domain.OutboundRelease, domain.FinalDelivery})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.restrictions.Save(txCtx, tenant, holding)
		return err
	})

	var duplicate ports.RestrictionSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.restrictions.Save(txCtx, tenant, holding)
		duplicate = saved
		return err
	})
	if duplicate != ports.RestrictionAlreadyRecorded {
		t.Fatalf("重复建立 outcome = %d, 想要 AlreadyRecorded", duplicate)
	}

	// 解除只凭责任来源接受的监管结果——领域单事件推进后 Update 落库。
	released, err := holding.ReleaseByRegulatoryOutcome(
		crgValue(t, domain.NewRegulatoryReleaseReference, "release/outcome-1"),
		crgBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("解除限制：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.restrictions.Update(txCtx, tenant, released)
	})

	found, exists, err := fixture.restrictions.FindByID(ctx, tenant,
		crgValue(t, domain.NewRestrictionID, "restriction-1"))
	if err != nil || !exists {
		t.Fatalf("读回限制：%v exists=%v", err, exists)
	}
	if found.Current() {
		t.Fatalf("解除后读回仍是有效限制")
	}
	release, at, hasRelease := found.Release()
	if !hasRelease || release.String() != "release/outcome-1" || !at.Equal(crgBaseAt.Add(time.Hour)) {
		t.Fatalf("解除依据没原样读回：%v %v %v", release, at, hasRelease)
	}
	if len(found.Constrains()) != 2 {
		t.Fatalf("约束集没原样读回：%+v", found.Constrains())
	}

	// 同范围第二份仍有效限制入列，异范围不进——ListByScope 是准入判断的读面。
	blocking := establishedRestriction(t, "restriction-2", "scope/parcel-1",
		[]domain.GuardedAction{domain.OutboundRelease})
	elsewhere := establishedRestriction(t, "restriction-3", "scope/parcel-9",
		[]domain.GuardedAction{domain.OutboundRelease})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.restrictions.Save(txCtx, tenant, blocking); err != nil {
			return err
		}
		_, err := fixture.restrictions.Save(txCtx, tenant, elsewhere)
		return err
	})
	listed, err := fixture.restrictions.ListByScope(ctx, tenant,
		crgValue(t, domain.NewDecisionScopeReference, "scope/parcel-1"))
	if err != nil {
		t.Fatalf("按范围盘限制：%v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("盘出 %d 份，想要 2（scope/parcel-9 的不该混进来）", len(listed))
	}
	admissibility, err := domain.JudgeActionAdmissibility(
		domain.OutboundRelease,
		crgValue(t, domain.NewDecisionScopeReference, "scope/parcel-1"),
		listed)
	if err != nil {
		t.Fatalf("准入判断：%v", err)
	}
	if admissibility.Admissible() || len(admissibility.BlockedBy()) != 1 {
		t.Fatalf("准入判断没按读回的限制阻断：%+v", admissibility.BlockedBy())
	}

	// 跨租户不可见；不存在的限制 Update 如实报错。
	if _, exists, err := fixture.restrictions.FindByID(ctx,
		crgValue(t, domain.NewTenantID, "tenant-b"),
		crgValue(t, domain.NewRestrictionID, "restriction-1")); err != nil || exists {
		t.Fatalf("跨租户可见：err=%v exists=%v", err, exists)
	}
	ghost := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.restrictions.Update(txCtx,
			crgValue(t, domain.NewTenantID, "tenant-b"), released)
	})
	if ghost == nil {
		t.Fatalf("另一租户拿同名标识改动了限制")
	}
}

// TestReleaseShapeIsPinnedByCheck 证解除形状入库内 CHECK：只有时刻没有依据的解除
// 被拦——released_at 非空必须监管结果引用同在场（IS NULL 显式分支过新纪律那一眼）。
func TestReleaseShapeIsPinnedByCheck(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at, released_by, released_at)
		 VALUES ('tenant-a', 'restriction-x', 'decision-1', 'scope/parcel-1',
		         '["OUTBOUND_RELEASE"]'::jsonb, now(), NULL, now())`); err == nil {
		t.Fatalf("没有监管结果引用的解除被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ('tenant-a', 'restriction-y', 'decision-1', 'scope/parcel-1', '[]'::jsonb, now())`); err == nil {
		t.Fatalf("空约束集被库接受了")
	}
}

func verifiedGate(t *testing.T, findings []domain.PreconditionFinding) (domain.ReleaseGateVerification, ports.GateVerificationKey) {
	t.Helper()
	scope := crgValue(t, domain.NewDecisionScopeReference, "scope/parcel-1")
	boundary := crgValue(t, domain.NewCustomsProcedureReference, "IMPORT_STANDARD")
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil {
		t.Fatalf("折门禁结论：%v", err)
	}
	preconditions := make([]domain.PreconditionReference, 0, len(findings))
	for _, finding := range findings {
		preconditions = append(preconditions, finding.Precondition)
	}
	gate, err := domain.VerifyReleaseGate(
		scope, domain.OutboundRelease, boundary, preconditions, conclusion, crgBaseAt)
	if err != nil {
		t.Fatalf("构造门禁核对：%v", err)
	}
	return gate, ports.GateVerificationKey{
		TenantID: crgValue(t, domain.NewTenantID, "tenant-a"),
		Scope:    scope,
		Action:   domain.OutboundRelease,
		Boundary: boundary,
		Digest:   ports.FindingsDigest(findings),
	}
}

// TestGateVerificationRoundTripsAndVersionsByDigest 证门禁往返与指纹换版：同指纹
// 第二次核对答`已有记录`不出第二版；条件状态变化换指纹即新行；`不适用`可以没有
// 前置条件清单（此动作在此边界本就不受门禁）。
func TestGateVerificationRoundTripsAndVersionsByDigest(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	findings := []domain.PreconditionFinding{
		{Precondition: crgValue(t, domain.NewPreconditionReference, "duty-paid"), State: domain.PreconditionMet},
		{Precondition: crgValue(t, domain.NewPreconditionReference, "restriction-clear"), State: domain.PreconditionUnmet},
	}
	gate, key := verifiedGate(t, findings)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, key, gate)
		return err
	})

	found, exists, err := fixture.gates.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("读回门禁核对：%v exists=%v", err, exists)
	}
	if found.Conclusion() != domain.GatePartiallyMet ||
		len(found.Preconditions()) != 2 ||
		!found.VerifiedAt().Equal(crgBaseAt) {
		t.Fatalf("门禁核对没原样读回：conclusion=%v preconditions=%d", found.Conclusion(), len(found.Preconditions()))
	}

	var duplicate ports.GateVerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.gates.Save(txCtx, key, gate)
		duplicate = saved
		return err
	})
	if duplicate != ports.GateVerificationAlreadyRecorded {
		t.Fatalf("同指纹重复核对 outcome = %d, 想要 AlreadyRecorded", duplicate)
	}

	// 条件状态变化：restriction-clear 转满足——指纹换了，是新版不是重放。
	changed := []domain.PreconditionFinding{
		findings[0],
		{Precondition: findings[1].Precondition, State: domain.PreconditionMet},
	}
	newGate, newKey := verifiedGate(t, changed)
	if newKey.Digest == key.Digest {
		t.Fatalf("条件状态变了指纹没变")
	}
	var reverified ports.GateVerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.gates.Save(txCtx, newKey, newGate)
		reverified = saved
		return err
	})
	if reverified != ports.GateVerificationSaved {
		t.Fatalf("新指纹 outcome = %d, 想要 Saved", reverified)
	}

	// 空清单即不适用——如实答案照样入库与读回。
	notApplicable, naKey := verifiedGate(t, nil)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, naKey, notApplicable)
		return err
	})
	foundNA, exists, err := fixture.gates.FindByKey(ctx, naKey)
	if err != nil || !exists {
		t.Fatalf("读回不适用门禁：%v exists=%v", err, exists)
	}
	if foundNA.Conclusion() != domain.GateNotApplicable || len(foundNA.Preconditions()) != 0 {
		t.Fatalf("不适用门禁没原样读回：%v", foundNA.Conclusion())
	}

	// 五值封闭入 CHECK：集合外结论的裸写进不来。
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.gate_verification
			(tenant_id, scope_ref, action, boundary_ref, findings_digest, preconditions, conclusion, verified_at)
		 VALUES ('tenant-a', 'scope/parcel-1', 'OUTBOUND_RELEASE', 'IMPORT_STANDARD',
		         'digest-x', '["p"]'::jsonb, 'MAYBE', now())`); err == nil {
		t.Fatalf("集合外门禁结论被库接受了")
	}
}

// TestCRGWritesRequireTransactionAndRollBack 证事务纪律：无事务写一律拒；事务失败
// 后案件、限制与门禁都不存在。
func TestCRGWritesRequireTransactionAndRollBack(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	key := caseKey(t, "tenant-a")
	customsCase := establishedCase(t, key, "case-1", nil)
	restriction := establishedRestriction(t, "restriction-1", "scope/parcel-1",
		[]domain.GuardedAction{domain.OutboundRelease})
	gate, gateKey := verifiedGate(t, nil)

	if _, err := fixture.cases.Save(ctx, key, customsCase); err == nil {
		t.Fatalf("无事务写案件被接受了")
	}
	if _, err := fixture.restrictions.Save(ctx, tenant, restriction); err == nil {
		t.Fatalf("无事务写限制被接受了")
	}
	if err := fixture.restrictions.Update(ctx, tenant, restriction); err == nil {
		t.Fatalf("无事务改限制被接受了")
	}
	if _, err := fixture.gates.Save(ctx, gateKey, gate); err == nil {
		t.Fatalf("无事务写门禁被接受了")
	}

	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.cases.Save(txCtx, key, customsCase); err != nil {
			return err
		}
		if _, err := fixture.restrictions.Save(txCtx, tenant, restriction); err != nil {
			return err
		}
		if _, err := fixture.gates.Save(txCtx, gateKey, gate); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}
	if _, exists, err := fixture.cases.FindByKey(ctx, key); err != nil || exists {
		t.Fatalf("回滚后案件仍在：err=%v exists=%v", err, exists)
	}
	if _, exists, err := fixture.restrictions.FindByID(ctx, tenant,
		crgValue(t, domain.NewRestrictionID, "restriction-1")); err != nil || exists {
		t.Fatalf("回滚后限制仍在：err=%v exists=%v", err, exists)
	}
	if _, exists, err := fixture.gates.FindByKey(ctx, gateKey); err != nil || exists {
		t.Fatalf("回滚后门禁仍在：err=%v exists=%v", err, exists)
	}
}
