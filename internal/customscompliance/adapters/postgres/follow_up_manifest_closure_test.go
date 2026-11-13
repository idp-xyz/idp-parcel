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

// 本文件对真实 PostgreSQL 16 证后续动作、舱单与关闭三库：目标建立代数、替代生效
// 只动三列、一舱单当前引用与版本推进、一案一份关闭且重开追加在记录内、形状 CHECK、
// 租户隔离由 SQL 条件承担。断言一律在事务闭包外。

var fmcBaseAt = time.Date(2026, 8, 14, 14, 0, 0, 0, time.UTC)

type fmcFixture struct {
	followUps  *adapter.FollowUps
	manifests  *adapter.Manifests
	closures   *adapter.CaseClosures
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newFMCFixture(t *testing.T) *fmcFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	followUps, err := adapter.NewFollowUps(db)
	if err != nil {
		t.Fatalf("构造后续动作库：%v", err)
	}
	manifests, err := adapter.NewManifests(db)
	if err != nil {
		t.Fatalf("构造舱单库：%v", err)
	}
	closures, err := adapter.NewCaseClosures(db)
	if err != nil {
		t.Fatalf("构造关闭库：%v", err)
	}
	return &fmcFixture{
		followUps: followUps, manifests: manifests, closures: closures,
		transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *fmcFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func fmcValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func followUpKey(t *testing.T, tenant string) ports.FollowUpTargetKey {
	t.Helper()
	return ports.FollowUpTargetKey{
		TenantID: fmcValue(t, domain.NewTenantID, tenant),
		Trigger:  fmcValue(t, domain.NewFollowUpTriggerReference, "regulatory-request/RR-9"),
		Version:  fmcValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Kind:     domain.ResubmissionReplacement,
	}
}

func formedTarget(t *testing.T) domain.FollowUpTarget {
	t.Helper()
	target, err := domain.FormFollowUpTarget(domain.FollowUpTargetSpec{
		Kind:     domain.ResubmissionReplacement,
		Trigger:  fmcValue(t, domain.NewFollowUpTriggerReference, "regulatory-request/RR-9"),
		CaseRef:  fmcValue(t, domain.NewCustomsCaseID, "case-1"),
		Unit:     fmcValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		Version:  fmcValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Scope:    fmcValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		FormedAt: fmcBaseAt,
	})
	if err != nil {
		t.Fatalf("构造后续目标：%v", err)
	}
	return target
}

func acceptedManifest(t *testing.T) domain.ExternalManifestReference {
	t.Helper()
	reference, err := domain.AcceptManifestReference(domain.ExternalManifestReferenceSpec{
		Manifest:   fmcValue(t, domain.NewExternalManifestID, "carrier-manifest/MAWB-123"),
		Version:    fmcValue(t, domain.NewManifestSourceVersion, "manifest/v1"),
		Carrier:    fmcValue(t, domain.NewCarrierResponsibilityReference, "carrier-1/regulatory-manifest"),
		Procedure:  fmcValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Direction:  domain.ImportManifest,
		Scope:      fmcValue(t, domain.NewDecisionScopeReference, "manifest-scope-1"),
		SourceFact: "carrier-report/submitted",
		AcceptedAt: fmcBaseAt,
	})
	if err != nil {
		t.Fatalf("接受舱单引用：%v", err)
	}
	return reference
}

func closedCase(t *testing.T) *domain.CustomsCaseClosure {
	t.Helper()
	return closedCaseWithRef(t, "case-1")
}

// closedCaseWithRef 走 VerifyClosure → CloseCase 真路径关一份指定引用的案件；两项义务一结一移交，够立起一份关闭。
func closedCaseWithRef(t *testing.T, caseRef string) *domain.CustomsCaseClosure {
	t.Helper()
	verification, err := domain.VerifyClosure(fmcValue(t, domain.NewCustomsCaseID, caseRef),
		fmcBaseAt, []domain.ClosureObligationItem{
			{
				Obligation: "declaration-submitted",
				Scope:      "declaration-unit-1",
				State:      domain.ObligationConcluded,
				Basis:      "basis/declaration-submitted",
			},
			{
				Obligation: "duty-settled",
				Scope:      "declaration-unit-1",
				State:      domain.ObligationHandedOver,
				Basis:      "basis/duty-settled",
				HandedTo:   "successor-team",
			},
		}, fmcBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("关闭核对：%v", err)
	}
	closure, err := domain.CloseCase(verification, "customs-owner", fmcBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("关闭案件：%v", err)
	}
	return closure
}

func (fixture *fmcFixture) saveTarget(t *testing.T, ctx context.Context, tenant string, target domain.FollowUpTarget) ports.FollowUpSaveOutcome {
	t.Helper()
	var outcome ports.FollowUpSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = fixture.followUps.SaveTarget(txCtx, followUpKey(t, tenant), target)
		return err
	})
	return outcome
}

func (fixture *fmcFixture) saveManifest(t *testing.T, ctx context.Context, tenant string, reference domain.ExternalManifestReference) ports.ManifestSaveOutcome {
	t.Helper()
	var outcome ports.ManifestSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = fixture.manifests.Save(txCtx, fmcValue(t, domain.NewTenantID, tenant), reference)
		return err
	})
	return outcome
}

func (fixture *fmcFixture) saveClosure(t *testing.T, ctx context.Context, tenant string, closure *domain.CustomsCaseClosure) ports.CaseClosureSaveOutcome {
	t.Helper()
	var outcome ports.CaseClosureSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = fixture.closures.Save(txCtx, fmcValue(t, domain.NewTenantID, tenant), closure)
		return err
	})
	return outcome
}

func TestFollowUpTargetRoundTripsAndSecondWriterIsAlreadyRecorded(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()

	target := formedTarget(t)
	if outcome := fixture.saveTarget(t, ctx, "tenant-a", target); outcome != ports.FollowUpSaved {
		t.Fatalf("首次 outcome = %d", outcome)
	}

	loaded, found, err := fixture.followUps.FindTarget(ctx, followUpKey(t, "tenant-a"))
	if err != nil || !found || loaded.CaseRef().String() != "case-1" || loaded.Kind() != domain.ResubmissionReplacement {
		t.Fatalf("往返失败：err=%v found=%v case=%s", err, found, loaded.CaseRef())
	}

	if outcome := fixture.saveTarget(t, ctx, "tenant-a", target); outcome != ports.FollowUpAlreadyRecorded {
		t.Fatalf("重写 outcome = %d", outcome)
	}
}

func TestReplacementProposeAndTakeEffectAreSeparateWrites(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	key := followUpKey(t, "tenant-a")

	fixture.saveTarget(t, ctx, "tenant-a", formedTarget(t))
	proposed, err := domain.ProposeReplacement(formedTarget(t),
		fmcValue(t, domain.NewDeclarationUnitID, "declaration-unit-2"))
	if err != nil {
		t.Fatalf("拟替代：%v", err)
	}

	var proposedOutcome ports.FollowUpSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		proposedOutcome, err = fixture.followUps.SaveRelation(txCtx, key, proposed)
		return err
	})
	if proposedOutcome != ports.FollowUpSaved {
		t.Fatalf("拟替代 outcome = %d", proposedOutcome)
	}

	loaded, found, err := fixture.followUps.FindRelation(ctx, key)
	if err != nil || !found || loaded.Effective() {
		t.Fatalf("拟替代往返：err=%v found=%v effective=%v", err, found, loaded.Effective())
	}

	effective, err := loaded.TakeEffect("external-result/authority-9", fmcBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("生效：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.followUps.UpdateRelation(txCtx, key, effective)
	})

	taken, found, err := fixture.followUps.FindRelation(ctx, key)
	if err != nil || !found || !taken.Effective() {
		t.Fatalf("生效往返：err=%v found=%v", err, found)
	}
	result, ok := taken.ExternalResult()
	if !ok || result != "external-result/authority-9" {
		t.Fatalf("外部结果 = %s ok=%v", result, ok)
	}

	var duplicateOutcome ports.FollowUpSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		duplicateOutcome, err = fixture.followUps.SaveRelation(txCtx, key, proposed)
		return err
	})
	if duplicateOutcome != ports.FollowUpAlreadyRecorded {
		t.Fatalf("同目标第二份关系 outcome = %d", duplicateOutcome)
	}
}

func TestManifestAcceptsThenRevisesTheCurrentRow(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")
	first := acceptedManifest(t)

	associated, err := first.Associate([]domain.AssociationCandidate{{
		Unit:      fmcValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		Procedure: first.Procedure(),
		Direction: first.Direction(),
		Scope:     first.Scope(),
	}})
	if err != nil {
		t.Fatalf("关联：%v", err)
	}
	if outcome := fixture.saveManifest(t, ctx, "tenant-a", associated); outcome != ports.ManifestSaved {
		t.Fatalf("首次 outcome = %d", outcome)
	}
	if outcome := fixture.saveManifest(t, ctx, "tenant-a", associated); outcome != ports.ManifestAlreadyRecorded {
		t.Fatalf("重写 outcome = %d", outcome)
	}

	loaded, found, err := fixture.manifests.FindByManifest(ctx, tenant, first.Carrier(), first.Manifest())
	if err != nil || !found {
		t.Fatalf("读首版：err=%v found=%v", err, found)
	}
	unit, ok := loaded.Association()
	if !ok || unit.String() != "declaration-unit-1" {
		t.Fatalf("关联丢了：%v ok=%v", unit, ok)
	}

	revised, err := loaded.Revise(
		fmcValue(t, domain.NewManifestSourceVersion, "manifest/v2"),
		fmcValue(t, domain.NewDecisionScopeReference, "manifest-scope-2"),
		"carrier-report/corrected",
		fmcBaseAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("改版：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		outcome, err := fixture.manifests.Update(txCtx, tenant, revised)
		if err == nil && outcome != ports.ManifestAdvanced {
			t.Errorf("推进 outcome = %d，want ManifestAdvanced", outcome)
		}
		return err
	})

	current, found, err := fixture.manifests.FindByManifest(ctx, tenant, first.Carrier(), first.Manifest())
	if err != nil || !found || current.Version().String() != "manifest/v2" {
		t.Fatalf("当前版 = %s found=%v err=%v", current.Version(), found, err)
	}
	prior, present := current.PriorVersion()
	if !present || prior.String() != "manifest/v1" {
		t.Fatalf("指回 = %s present=%v", prior, present)
	}
	if _, associated := current.Association(); associated {
		t.Fatal("新版本把旧关联自动搬过来了")
	}

	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM customs_compliance.external_manifest_reference
		  WHERE tenant_id = 'tenant-a'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 1 {
		t.Fatalf("舱单行数 = %d，应只管当前引用", rows)
	}
}

func TestCaseClosureRoundTripsAndReopeningAppendsInPlace(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")
	closure := closedCase(t)

	if outcome := fixture.saveClosure(t, ctx, "tenant-a", closure); outcome != ports.CaseClosureSaved {
		t.Fatalf("首次 outcome = %d", outcome)
	}
	if outcome := fixture.saveClosure(t, ctx, "tenant-a", closure); outcome != ports.CaseClosureAlreadyRecorded {
		t.Fatalf("重关 outcome = %d", outcome)
	}

	loaded, found, err := fixture.closures.FindByCase(ctx, tenant,
		fmcValue(t, domain.NewCustomsCaseID, "case-1"))
	if err != nil || !found || loaded.DecidedBy() != "customs-owner" {
		t.Fatalf("关闭往返：err=%v found=%v", err, found)
	}

	if err := closure.Reopen(domain.ControlledReopening{
		LateFact:      "late-regulatory-correction/9",
		AffectedItems: []string{"declaration-submitted"},
		Authority:     "customs-owner",
		ReopenedAt:    fmcBaseAt.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("重开：%v", err)
	}
	if outcome := fixture.saveClosure(t, ctx, "tenant-a", closure); outcome != ports.CaseClosureSaved {
		t.Fatalf("重开落库 outcome = %d", outcome)
	}

	reopened, found, err := fixture.closures.FindByCase(ctx, tenant,
		fmcValue(t, domain.NewCustomsCaseID, "case-1"))
	if err != nil || !found {
		t.Fatalf("重开后读取：err=%v found=%v", err, found)
	}
	if !reopened.ClosedAt().Equal(loaded.ClosedAt()) {
		t.Fatal("重开改写了原关闭时间")
	}
	if len(reopened.Reopenings()) != 1 || reopened.Reopenings()[0].LateFact != "late-regulatory-correction/9" {
		t.Fatalf("重开记录 = %#v", reopened.Reopenings())
	}
}

func TestFollowUpManifestClosureChecksRejectBadShapes(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.follow_up_target
			(tenant_id, trigger_ref, version_id, kind, case_ref, unit_id, scope_ref, formed_at)
		 VALUES ('t', 'tr', 'v', 'TECHNICAL_RETRY', 'c', 'u', 's', now())`); err == nil {
		t.Fatal("封闭集合外的动作种类被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.follow_up_target
			(tenant_id, trigger_ref, version_id, kind, case_ref, unit_id, scope_ref, formed_at)
		 VALUES ('t', 'tr', 'v', 'RESUBMISSION_REPLACEMENT', 'c', 'u', 's', now())`); err != nil {
		t.Fatalf("为形状探针准备目标：%v", err)
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.follow_up_replacement
			(tenant_id, trigger_ref, version_id, kind, replacement_unit, effective)
		 VALUES ('t', 'tr', 'v', 'RESUBMISSION_REPLACEMENT', 'u2', true)`); err == nil {
		t.Fatal("已生效却无外部结果的替代被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.external_manifest_reference
			(tenant_id, manifest_id, version_id, carrier_ref, procedure_ref, direction,
			 scope_ref, source_fact, accepted_at)
		 VALUES ('t', 'm', 'v', 'c', 'p', 'TRANSIT', 's', 'f', now())`); err == nil {
		t.Fatal("封闭集合外的舱单方向被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO customs_compliance.case_closure
			(tenant_id, case_ref, cutoff_at, verified_at, decided_by, closed_at, items)
		 VALUES ('t', 'c', now(), now(), 'd', now(), '[]')`); err == nil {
		t.Fatal("空义务清单的关闭被库接受了")
	}
}

func TestFollowUpManifestClosuresOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()

	fixture.saveTarget(t, ctx, "tenant-a", formedTarget(t))
	fixture.saveManifest(t, ctx, "tenant-a", acceptedManifest(t))
	fixture.saveClosure(t, ctx, "tenant-a", closedCase(t))

	if _, found, err := fixture.followUps.FindTarget(ctx, followUpKey(t, "tenant-b")); err != nil || found {
		t.Fatalf("跨租户目标可见：err=%v found=%v", err, found)
	}
	if _, found, err := fixture.manifests.FindByManifest(ctx,
		fmcValue(t, domain.NewTenantID, "tenant-b"), acceptedManifest(t).Carrier(),
		fmcValue(t, domain.NewExternalManifestID, "carrier-manifest/MAWB-123")); err != nil || found {
		t.Fatalf("跨租户舱单可见：err=%v found=%v", err, found)
	}
	if _, found, err := fixture.closures.FindByCase(ctx,
		fmcValue(t, domain.NewTenantID, "tenant-b"),
		fmcValue(t, domain.NewCustomsCaseID, "case-1")); err != nil || found {
		t.Fatalf("跨租户关闭可见：err=%v found=%v", err, found)
	}

	fixture.saveTarget(t, ctx, "tenant-b", formedTarget(t))
	fixture.saveManifest(t, ctx, "tenant-b", acceptedManifest(t))
	fixture.saveClosure(t, ctx, "tenant-b", closedCase(t))
}

func TestFollowUpManifestClosureWritesRequireTransaction(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")

	if _, err := fixture.followUps.SaveTarget(ctx, followUpKey(t, "tenant-a"), formedTarget(t)); err == nil {
		t.Fatal("无事务 SaveTarget 被接受了")
	}
	if _, err := fixture.manifests.Save(ctx, tenant, acceptedManifest(t)); err == nil {
		t.Fatal("无事务 Save 舱单被接受了")
	}
	if _, err := fixture.closures.Save(ctx, tenant, closedCase(t)); err == nil {
		t.Fatal("无事务 Save 关闭被接受了")
	}
}

func TestReplacementCannotExistBeforeTarget(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	key := followUpKey(t, "tenant-a")
	proposed, err := domain.ProposeReplacement(formedTarget(t),
		fmcValue(t, domain.NewDeclarationUnitID, "declaration-unit-2"))
	if err != nil {
		t.Fatalf("拟替代：%v", err)
	}

	err = fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.followUps.SaveRelation(txCtx, key, proposed)
		return err
	})
	if err == nil {
		t.Fatal("没有目标的替代关系被库接受了")
	}
}

// Covers: 迁移 customs_compliance 0034 的键与 Update 的前版条件（UC-CC-012）——两家承运商用同一个舱单号各自成行，谁也
// 占不住谁的号、读不到谁的引用；基于同一份前版的两笔推进，先落的一笔成，后落的一笔答版本已移动，不盖掉先落的。
func TestTwoCarriersHoldTheSameManifestNumberAndAStaleRevisionDoesNotOverwrite(t *testing.T) {
	fixture := newFMCFixture(t)
	ctx := t.Context()
	tenant := fmcValue(t, domain.NewTenantID, "tenant-a")
	first := acceptedManifest(t)
	spec := domain.ExternalManifestReferenceSpec{
		Manifest:   first.Manifest(),
		Version:    fmcValue(t, domain.NewManifestSourceVersion, "other/v1"),
		Carrier:    fmcValue(t, domain.NewCarrierResponsibilityReference, "carrier-responsibility/OTHER"),
		Procedure:  first.Procedure(),
		Direction:  first.Direction(),
		Scope:      fmcValue(t, domain.NewDecisionScopeReference, "other-scope-1"),
		SourceFact: "other-report/submitted",
		AcceptedAt: fmcBaseAt,
	}
	second, err := domain.AcceptManifestReference(spec)
	if err != nil {
		t.Fatalf("接受第二家的引用：%v", err)
	}
	if first.Carrier() == second.Carrier() {
		t.Fatal("夹具没造对：两份引用要属于不同的承运商责任")
	}
	for name, reference := range map[string]domain.ExternalManifestReference{"第一家": first, "第二家": second} {
		if outcome := fixture.saveManifest(t, ctx, "tenant-a", reference); outcome != ports.ManifestSaved {
			t.Fatalf("%s首登 outcome = %d，want ManifestSaved——同一个号在另一家的编号空间里是另一行", name, outcome)
		}
	}
	loaded, found, err := fixture.manifests.FindByManifest(ctx, tenant, second.Carrier(), second.Manifest())
	if err != nil || !found || loaded.Version().String() != "other/v1" || loaded.Scope().String() != "other-scope-1" {
		t.Fatalf("第二家读回的不是自己的引用：%+v found=%v err=%v", loaded, found, err)
	}

	advance := func(version, scope string) ports.ManifestUpdateOutcome {
		t.Helper()
		revised, err := first.Revise(
			fmcValue(t, domain.NewManifestSourceVersion, version),
			fmcValue(t, domain.NewDecisionScopeReference, scope),
			"carrier-report/"+version, fmcBaseAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("改版 %s：%v", version, err)
		}
		var outcome ports.ManifestUpdateOutcome
		fixture.inTx(t, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = fixture.manifests.Update(txCtx, tenant, revised)
			return err
		})
		return outcome
	}
	if outcome := advance("manifest/v2", "manifest-scope-2"); outcome != ports.ManifestAdvanced {
		t.Fatalf("先落的推进 outcome = %d，want ManifestAdvanced", outcome)
	}
	if outcome := advance("manifest/v3", "manifest-scope-3"); outcome != ports.ManifestVersionMoved {
		t.Fatalf("基于旧前版的推进 outcome = %d，want ManifestVersionMoved", outcome)
	}
	current, _, err := fixture.manifests.FindByManifest(ctx, tenant, first.Carrier(), first.Manifest())
	if err != nil || current.Version().String() != "manifest/v2" {
		t.Fatalf("在册版本 = %q err=%v，v2 被后到的推进盖掉了", current.Version(), err)
	}
	untouched, _, err := fixture.manifests.FindByManifest(ctx, tenant, second.Carrier(), second.Manifest())
	if err != nil || untouched.Version().String() != "other/v1" {
		t.Fatalf("第二家的引用被第一家的推进动了：%q err=%v", untouched.Version(), err)
	}
}
