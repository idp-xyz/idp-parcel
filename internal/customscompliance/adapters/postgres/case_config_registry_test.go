package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 五本案件配置登记册的写口用例。两条贯穿全篇的做法：
//
//   - **断言穿过对应的只读视图取回**，不自己 SELECT。写口与读口在列名、时区或枚举串
//     上对不上时，两边的单测各自都能绿，只有往返才露馅。
//   - **写入一律放进环境事务**，照生产编排的调法调。写口走 RequireExecutor，无事务
//     即报错；绕开这道门禁的测试会把「登记能与它的交接事件同笔落地」这件事测没了。
//
// 用例覆盖的是三条纪律（见 case_config_registry.go 头注）——不可覆盖、撤销走状态推进、
// 目录与明细分开。它们都属于「没有测试就会被下一个人顺手改掉」的那类。

var registryBaseAt = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

// register 在事务里跑一次登记，并把结果代数带出来。内层返回错误时事务回滚，与生产
// 一致：登记失败不该留下半行。
func register(
	t *testing.T,
	fixture *viewFixture,
	call func(context.Context) (ports.CaseConfigurationSaveOutcome, error),
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	var outcome ports.CaseConfigurationSaveOutcome
	err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var callErr error
		outcome, callErr = call(txCtx)
		return callErr
	})
	return outcome, err
}

// mutate 在事务里跑一次只返回错误的写入（撤销那一族）。
func mutate(t *testing.T, fixture *viewFixture, call func(context.Context) error) error {
	t.Helper()
	return fixture.db.Transactor().WithinTransaction(t.Context(), call)
}

func newReadinessRegistry(t *testing.T) (*adapter.ReadinessRegistrations, *adapter.ReadinessView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewReadinessRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪写口：%v", err)
	}
	view, err := adapter.NewReadinessView(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪读口：%v", err)
	}
	return registry, view, fixture
}

func readyJudgment(t *testing.T, unit, basis string) domain.ReadinessJudgment {
	t.Helper()
	judgment, err := domain.JudgeReady(
		viewValue(t, domain.NewDeclarationUnitID, unit),
		viewValue(t, domain.NewReadinessBasisReference, basis),
		registryBaseAt)
	if err != nil {
		t.Fatalf("形成就绪判断：%v", err)
	}
	return judgment
}

func TestRegisteredReadinessIsReadBackThroughItsView(t *testing.T) {
	registry, view, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3"))
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("登记就绪：err=%v outcome=%v", err, outcome)
	}

	loaded, found, err := view.LoadReadiness(t.Context(), tenant, viewValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil || !found {
		t.Fatalf("就绪没读回：err=%v found=%v", err, found)
	}
	if loaded.Basis().String() != "DOSSIER/COMPLETE-V3" || !loaded.JudgedAt().Equal(registryBaseAt) || !loaded.Effective() {
		t.Fatalf("往返走样：basis=%s judgedAt=%s effective=%v",
			loaded.Basis(), loaded.JudgedAt(), loaded.Effective())
	}
}

// 写口参与调用方的事务：回滚之后一行都不该留下。没有这一条，登记就无法与它的交接
// 事件同笔落地，而那正是本上下文其余写口共同守的形状。
func TestRegistrationRollsBackWithItsTransaction(t *testing.T) {
	registry, view, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	rollback := errors.New("回滚")

	err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := registry.RegisterReadiness(txCtx, tenant, readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3")); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, found, err := view.LoadReadiness(t.Context(), tenant,
		viewValue(t, domain.NewDeclarationUnitID, "unit-1")); err != nil || found {
		t.Fatalf("回滚后登记仍在：err=%v found=%v", err, found)
	}
}

// 无环境事务时写口拒绝执行，而不是自己开一笔。自开事务会让登记在调用方回滚后独自
// 留存——配置就与它所属的那次业务动作脱钩了。
func TestRegistrationRefusesToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newReadinessRegistry(t)
	if _, err := registry.RegisterReadiness(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3")); err == nil {
		t.Fatal("没有环境事务时写口仍然写了进去")
	}
}

// 不可覆盖那条纪律最尖的一格：换了依据再登记，交回`已登记`，且**库里仍是原依据**。
// 若哪天有人把写口改成 UPSERT，上面的往返用例照样绿，只有这一条会红。
func TestReRegisteringReadinessNeverReplacesTheRecordedBasis(t *testing.T) {
	registry, view, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, readyJudgment(t, "unit-1", "DOSSIER/FIRST"))
	}); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, readyJudgment(t, "unit-1", "DOSSIER/SECOND"))
	})
	if err != nil {
		t.Fatalf("二次登记：%v", err)
	}
	if outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("二次登记该交回`已登记`，得到 %v", outcome)
	}

	loaded, _, err := view.LoadReadiness(t.Context(), tenant, viewValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if loaded.Basis().String() != "DOSSIER/FIRST" {
		t.Fatalf("原依据被顶替成 %s", loaded.Basis())
	}
}

// 撤销是同一行的状态推进：读口仍找得到这一行，原依据与形成时间原样在，只是不再有效。
// 若撤销被写成删行，found 会变 false——那时「已撤销」就与「从未登记」分不开了。
func TestRevokingReadinessKeepsTheOriginalJudgmentReadable(t *testing.T) {
	registry, view, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	judgment := readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, judgment)
	}); err != nil {
		t.Fatalf("登记：%v", err)
	}
	revoked, err := judgment.Revoke("RULE/CHANGED", registryBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("形成撤销：%v", err)
	}
	if err := mutate(t, fixture, func(ctx context.Context) error {
		return registry.RevokeReadiness(ctx, tenant, revoked)
	}); err != nil {
		t.Fatalf("撤销：%v", err)
	}

	loaded, found, err := view.LoadReadiness(t.Context(), tenant, viewValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil || !found {
		t.Fatalf("撤销后这一行不见了：err=%v found=%v", err, found)
	}
	if loaded.Effective() {
		t.Fatal("撤销没写进库，读回仍然有效")
	}
	if loaded.Basis().String() != "DOSSIER/COMPLETE-V3" || !loaded.JudgedAt().Equal(registryBaseAt) {
		t.Fatalf("撤销顺手改掉了原判断：basis=%s judgedAt=%s", loaded.Basis(), loaded.JudgedAt())
	}
}

// 重复撤销撞不动第一次的原因与时间：WHERE 带 revoked_at IS NULL，谁先撤销成功谁算。
func TestASecondRevocationDoesNotOverwriteTheFirst(t *testing.T) {
	registry, _, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	judgment := readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, judgment)
	}); err != nil {
		t.Fatalf("登记：%v", err)
	}
	first, err := judgment.Revoke("RULE/CHANGED", registryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成首次撤销：%v", err)
	}
	if err := mutate(t, fixture, func(ctx context.Context) error {
		return registry.RevokeReadiness(ctx, tenant, first)
	}); err != nil {
		t.Fatalf("首次撤销：%v", err)
	}

	second, err := judgment.Revoke("CREDENTIAL/EXPIRED", registryBaseAt.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("形成二次撤销：%v", err)
	}
	if err := mutate(t, fixture, func(ctx context.Context) error {
		return registry.RevokeReadiness(ctx, tenant, second)
	}); err == nil {
		t.Fatal("二次撤销被静默接受，第一次的原因与时间有被顶替的风险")
	}
}

// 已撤销的判断不得从登记口整行写入：那会把「先就绪后失效」压成「一进来就是失效的」，
// 两者在审计上不是一回事。
func TestARevokedJudgmentCannotEnterThroughTheRegistrationPort(t *testing.T) {
	registry, _, fixture := newReadinessRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")

	revoked, err := readyJudgment(t, "unit-1", "DOSSIER/COMPLETE-V3").
		Revoke("RULE/CHANGED", registryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成撤销：%v", err)
	}
	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterReadiness(ctx, tenant, revoked)
	})
	if err == nil {
		t.Fatal("登记口收下了一条已撤销的判断")
	}
	if outcome != ports.CaseConfigurationSaveOutcomeInvalid {
		t.Fatalf("该交回`非法`，得到 %v", outcome)
	}
}

func TestSubmissionAuthorityRegistrationRoundTripsAndRevokesInPlace(t *testing.T) {
	fixture := newViewFixture(t)
	registry, err := adapter.NewSubmissionAuthorityRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造授权写口：%v", err)
	}
	view, err := adapter.NewSubmissionAuthorityView(fixture.db)
	if err != nil {
		t.Fatalf("构造授权读口：%v", err)
	}
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	unit := viewValue(t, domain.NewDeclarationUnitID, "unit-1")

	authorization, err := domain.GrantSubmissionAuthority(unit,
		viewValue(t, domain.NewSubmissionAuthorityReference, "POA/ACME-2026"), registryBaseAt)
	if err != nil {
		t.Fatalf("形成授权：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.GrantSubmissionAuthority(ctx, tenant, authorization)
	}); err != nil {
		t.Fatalf("授予：%v", err)
	}

	loaded, found, err := view.LoadSubmissionAuthority(t.Context(), tenant, unit)
	if err != nil || !found || loaded.Authority().String() != "POA/ACME-2026" || !loaded.Effective() {
		t.Fatalf("授权往返走样：err=%v found=%v %+v", err, found, loaded)
	}

	revoked, err := authorization.Revoke("DELEGATION/WITHDRAWN", registryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成撤销：%v", err)
	}
	if err := mutate(t, fixture, func(ctx context.Context) error {
		return registry.RevokeSubmissionAuthority(ctx, tenant, revoked)
	}); err != nil {
		t.Fatalf("撤销授权：%v", err)
	}

	loaded, found, err = view.LoadSubmissionAuthority(t.Context(), tenant, unit)
	if err != nil || !found {
		t.Fatalf("撤销后这一行不见了：err=%v found=%v", err, found)
	}
	if loaded.Effective() || loaded.Authority().String() != "POA/ACME-2026" {
		t.Fatalf("撤销那一格走样：effective=%v authority=%s", loaded.Effective(), loaded.Authority())
	}
}

// 就绪与授权分表分口那条：撤销了授权不该连带动就绪那一轨（CONTEXT「提交授权与就绪判断分别形成和失效」）。
func TestRevokingAuthorityLeavesReadinessUntouched(t *testing.T) {
	fixture := newViewFixture(t)
	readiness, err := adapter.NewReadinessRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪写口：%v", err)
	}
	authorities, err := adapter.NewSubmissionAuthorityRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造授权写口：%v", err)
	}
	readinessView, err := adapter.NewReadinessView(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪读口：%v", err)
	}
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	unit := viewValue(t, domain.NewDeclarationUnitID, "unit-1")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return readiness.RegisterReadiness(ctx, tenant, readyJudgment(t, "unit-1", "DOSSIER/COMPLETE"))
	}); err != nil {
		t.Fatalf("登记就绪：%v", err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unit,
		viewValue(t, domain.NewSubmissionAuthorityReference, "POA/ACME-2026"), registryBaseAt)
	if err != nil {
		t.Fatalf("形成授权：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return authorities.GrantSubmissionAuthority(ctx, tenant, authorization)
	}); err != nil {
		t.Fatalf("授予：%v", err)
	}
	revoked, err := authorization.Revoke("DELEGATION/WITHDRAWN", registryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成撤销：%v", err)
	}
	if err := mutate(t, fixture, func(ctx context.Context) error {
		return authorities.RevokeSubmissionAuthority(ctx, tenant, revoked)
	}); err != nil {
		t.Fatalf("撤销授权：%v", err)
	}

	loaded, found, err := readinessView.LoadReadiness(t.Context(), tenant, unit)
	if err != nil || !found || !loaded.Effective() {
		t.Fatalf("授权失效连带动了就绪：err=%v found=%v effective=%v", err, found, loaded.Effective())
	}
}

func newInterpretationRuleRegistry(t *testing.T) (*adapter.InterpretationRuleRegistrations, *adapter.InterpretationRuleView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewInterpretationRuleRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造解释规则写口：%v", err)
	}
	view, err := adapter.NewInterpretationRuleView(fixture.db)
	if err != nil {
		t.Fatalf("构造解释规则读口：%v", err)
	}
	return registry, view, fixture
}

// registerRule 是解释规则登记的短手：同支四键常量在每个用例里重复太吵。
func registerRule(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.InterpretationRuleRegistrations,
	rule string,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterInterpretationRule(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
			viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
			viewValue(t, domain.NewInterpretationRuleReference, rule), appliesFrom)
	})
}

func loadRuleAt(
	t *testing.T,
	view *adapter.InterpretationRuleView,
	evaluatedAt time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	t.Helper()
	return view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"), evaluatedAt)
}

// 同键（同支同起点）换规则交回`已登记`，库里仍是原规则——顶替会让既有 ExternalResult
// 上「实际采用的规则」失去依据。多版本形状下 W13 的不可覆盖语义原样保持。
func TestRegisteringAnotherRuleAtTheSameStartNeverReplacesTheFirst(t *testing.T) {
	registry, view, fixture := newInterpretationRuleRegistry(t)

	if _, err := registerRule(t, fixture, registry, "interpret/release/v1", registryBaseAt); err != nil {
		t.Fatalf("首次登记：%v", err)
	}
	outcome, err := registerRule(t, fixture, registry, "interpret/release/v2", registryBaseAt)
	if err != nil {
		t.Fatalf("二次登记：%v", err)
	}
	if outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键换规则该交回`已登记`由编排判冲突，得到 %v", outcome)
	}

	rule, found, err := loadRuleAt(t, view, registryBaseAt)
	if err != nil || !found || rule.String() != "interpret/release/v1" {
		t.Fatalf("原规则被顶替：err=%v found=%v rule=%s", err, found, rule)
	}
}

// 换版（ADR-0070 支点场景的登记半边）：登记更晚起点的新版给开放前版落终点。前版的
// 规则与起点原样留在行内，只有终点从 NULL 走到后继起点——按业务发生时间落在旧区间的
// 迟到响应仍解析回旧版，绝不是到达时刻的当前指针。
func TestSupersedingClosesThePredecessorAndOldInstantsResolveTheOldRule(t *testing.T) {
	registry, view, fixture := newInterpretationRuleRegistry(t)
	successionAt := registryBaseAt.Add(48 * time.Hour)

	if _, err := registerRule(t, fixture, registry, "interpret/release/v1", registryBaseAt); err != nil {
		t.Fatalf("登记 v1：%v", err)
	}
	outcome, err := registerRule(t, fixture, registry, "interpret/release/v2", successionAt)
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("换版该是新登记：err=%v outcome=%v", err, outcome)
	}

	early, foundEarly, err := loadRuleAt(t, view, registryBaseAt.Add(time.Hour))
	if err != nil || !foundEarly || early.String() != "interpret/release/v1" {
		t.Fatalf("旧区间时点没解析回 v1：err=%v found=%v rule=%s", err, foundEarly, early)
	}
	late, foundLate, err := loadRuleAt(t, view, successionAt)
	if err != nil || !foundLate || late.String() != "interpret/release/v2" {
		t.Fatalf("换版起点没解析到 v2：err=%v found=%v rule=%s", err, foundLate, late)
	}

	// 前版行上只有终点动了：规则与起点原样，终点恰为后继起点（状态推进，不是覆盖）。
	var storedRule string
	var appliesUntil time.Time
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT rule_ref, applies_until FROM customs_compliance.interpretation_rule
		  WHERE tenant_id = 'tenant-a' AND result_layer = 'RELEASE_RESULT'
		    AND jurisdiction_ref = 'JURIS/DE' AND applies_from = $1`,
		registryBaseAt).Scan(&storedRule, &appliesUntil); err != nil {
		t.Fatalf("读前版行：%v", err)
	}
	if storedRule != "interpret/release/v1" || !appliesUntil.Equal(successionAt) {
		t.Fatalf("前版行走样：rule=%s until=%s", storedRule, appliesUntil)
	}
}

// 起点早于既有版本覆盖面的开放登记撞上排他约束：交回`已登记`且一行未写——历史区间
// 是已记录的选择依据，不接受被一次错序登记追改。
func TestABackdatedOpenRegistrationIsHeldByTheOverlapGuard(t *testing.T) {
	registry, view, fixture := newInterpretationRuleRegistry(t)

	if _, err := registerRule(t, fixture, registry, "interpret/release/v2", registryBaseAt); err != nil {
		t.Fatalf("登记 v2：%v", err)
	}
	outcome, err := registerRule(t, fixture, registry, "interpret/release/v1",
		registryBaseAt.Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("错序登记：%v", err)
	}
	if outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("错序登记该被重叠约束折成`已登记`，得到 %v", outcome)
	}
	// 按请求起点读不回任何版本——编排据此判冲突，而不是把错序静默读成重放。
	if _, found, err := loadRuleAt(t, view, registryBaseAt.Add(-48*time.Hour)); err != nil || found {
		t.Fatalf("错序登记竟然可解析：err=%v found=%v", err, found)
	}
}

// 分层保存在写口这一侧的样子：登了放行层不等于登了处置层；辖区维同理——JURIS/DE 的
// 版本不替 JURIS/US 作答（多辖区租户正是版本维要接住的那半发作面）。
func TestRegistrationsDoNotAnswerAcrossLayersOrJurisdictions(t *testing.T) {
	registry, view, fixture := newInterpretationRuleRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")

	if _, err := registerRule(t, fixture, registry, "interpret/release/v1", registryBaseAt); err != nil {
		t.Fatalf("登记：%v", err)
	}
	if _, found, err := view.LoadInterpretationRule(t.Context(), tenant, domain.DispositionDecisionLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
		registryBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("处置层被放行层的登记顺带配上了：err=%v found=%v", err, found)
	}
	if _, found, err := view.LoadInterpretationRule(t.Context(), tenant, domain.ReleaseResultLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/US"),
		registryBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("另一辖区被顺带配上了：err=%v found=%v", err, found)
	}
}

// 封闭六层之外的层与零值生效起点都不静默写成一行：那是调用方的编程错误，与「实例还
// 没登记」两回事。
func TestRegisteringAnUnknownLayerOrZeroStartIsLoud(t *testing.T) {
	registry, _, fixture := newInterpretationRuleRegistry(t)
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterInterpretationRule(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"), domain.ResultLayerInvalid,
			viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
			viewValue(t, domain.NewInterpretationRuleReference, "interpret/x"), registryBaseAt)
	}); err == nil {
		t.Fatal("非法结果层被静默登记")
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterInterpretationRule(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
			viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
			viewValue(t, domain.NewInterpretationRuleReference, "interpret/x"), time.Time{})
	}); err == nil {
		t.Fatal("零值生效起点被静默登记")
	}
}

func newObligationRegistry(t *testing.T) (*adapter.ObligationInventoryRegistrations, *adapter.ObligationInventoryView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewObligationInventoryRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造义务写口：%v", err)
	}
	view, err := adapter.NewObligationInventoryView(fixture.db)
	if err != nil {
		t.Fatalf("构造义务读口：%v", err)
	}
	return registry, view, fixture
}

func registerObligationCatalog(t *testing.T, registry *adapter.ObligationInventoryRegistrations, fixture *viewFixture, tenant domain.TenantID, caseRef string) {
	t.Helper()
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterObligationCatalog(ctx, tenant,
			viewValue(t, domain.NewCustomsCaseID, caseRef), registryBaseAt)
	}); err != nil {
		t.Fatalf("登记义务目录：%v", err)
	}
}

// 目录与明细分两个方法的理由，在往返上验一次：只登目录不登明细，读口答的是
// 「已登记且本截点空清单」——即「此案在此截点无适用义务」，不是未决。
// 这一格若表达不出来，可关的案件就只能靠「查不到」冒充，等于把关闭判断放开。
func TestRegisteringOnlyTheCatalogYieldsAnEmptyInventoryRatherThanUnconfigured(t *testing.T) {
	registry, view, fixture := newObligationRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	registerObligationCatalog(t, registry, fixture, tenant, "case-1")

	items, configured, err := view.LoadObligationItems(t.Context(), tenant,
		viewValue(t, domain.NewCustomsCaseID, "case-1"), registryBaseAt.Add(time.Hour))
	if err != nil || !configured {
		t.Fatalf("登了目录却答未配置：err=%v configured=%v", err, configured)
	}
	if len(items) != 0 {
		t.Fatalf("空清单里冒出了 %d 项", len(items))
	}
}

// 明细的适用区间随项登记，读口按业务截点半开区间盘点：区间外的项不进本次清单，
// 但它仍在册——「此刻不适用」与「没登记」是两回事。
func TestObligationItemsEnterTheInventoryOnlyWithinTheirInterval(t *testing.T) {
	registry, view, fixture := newObligationRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	registerObligationCatalog(t, registry, fixture, tenant, "case-1")

	expiring := ports.ObligationRegistration{
		Item: domain.ClosureObligationItem{
			Obligation: "DUTY/PAYMENT",
			Scope:      "case-1",
			State:      domain.ObligationUnresolved,
			Basis:      "PROGRAM/DDP",
		},
		AppliesFrom:  registryBaseAt,
		AppliesUntil: registryBaseAt.Add(4 * time.Hour),
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterObligationItem(ctx, tenant,
			viewValue(t, domain.NewCustomsCaseID, "case-1"), expiring)
	}); err != nil {
		t.Fatalf("登记限期义务：%v", err)
	}

	items, _, err := view.LoadObligationItems(t.Context(), tenant,
		viewValue(t, domain.NewCustomsCaseID, "case-1"), registryBaseAt.Add(time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatalf("区间内没盘出来：err=%v items=%d", err, len(items))
	}
	// 半开区间：终点当刻已不再适用。
	items, configured, err := view.LoadObligationItems(t.Context(), tenant,
		viewValue(t, domain.NewCustomsCaseID, "case-1"), registryBaseAt.Add(4*time.Hour))
	if err != nil || !configured {
		t.Fatalf("盘点：err=%v configured=%v", err, configured)
	}
	if len(items) != 0 {
		t.Fatal("终点当刻仍把该项盘了进来")
	}
}

// AppliesUntil 零值必须落成 NULL 而不是 0001 年：写成零时刻的话，这一项在任何真实
// 截点上都已「失效」，登记了却永远盘不进来，而且不会有任何东西报错。
func TestAnObligationWithNoEndRemainsApplicableFarInTheFuture(t *testing.T) {
	registry, view, fixture := newObligationRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	registerObligationCatalog(t, registry, fixture, tenant, "case-1")

	openEnded := ports.ObligationRegistration{
		Item: domain.ClosureObligationItem{
			Obligation: "RECORD/RETENTION",
			Scope:      "case-1",
			State:      domain.ObligationUnresolved,
			Basis:      "PROGRAM/ARCHIVE",
		},
		AppliesFrom: registryBaseAt,
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterObligationItem(ctx, tenant,
			viewValue(t, domain.NewCustomsCaseID, "case-1"), openEnded)
	}); err != nil {
		t.Fatalf("登记无终点义务：%v", err)
	}

	items, _, err := view.LoadObligationItems(t.Context(), tenant,
		viewValue(t, domain.NewCustomsCaseID, "case-1"), registryBaseAt.AddDate(5, 0, 0))
	if err != nil || len(items) != 1 {
		t.Fatalf("无终点的义务在五年后盘不出来了：err=%v items=%d", err, len(items))
	}
}

// 承接项的接收责任方随项往返（CONTEXT「来源责任方、接收责任方、接受决定及权限」）；非承接项写 NULL，否则库的双向
// CHECK 会把空串当成「带了一个空名字」挡下。
func TestAHandedOverObligationCarriesItsRecipientBackThroughTheView(t *testing.T) {
	registry, view, fixture := newObligationRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	registerObligationCatalog(t, registry, fixture, tenant, "case-1")

	for _, registration := range []ports.ObligationRegistration{
		{
			Item: domain.ClosureObligationItem{
				Obligation: "BROKER/FILING",
				Scope:      "case-1",
				State:      domain.ObligationHandedOver,
				Basis:      "CONTRACT/BROKER-A",
				HandedTo:   "BROKER/ACME",
			},
			AppliesFrom: registryBaseAt,
		},
		{
			Item: domain.ClosureObligationItem{
				Obligation: "DUTY/PAYMENT",
				Scope:      "case-1",
				State:      domain.ObligationConcluded,
				Basis:      "RECEIPT/PAID",
			},
			AppliesFrom: registryBaseAt,
		},
	} {
		if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return registry.RegisterObligationItem(ctx, tenant,
				viewValue(t, domain.NewCustomsCaseID, "case-1"), registration)
		}); err != nil {
			t.Fatalf("登记义务 %s：%v", registration.Item.Obligation, err)
		}
	}

	items, _, err := view.LoadObligationItems(t.Context(), tenant,
		viewValue(t, domain.NewCustomsCaseID, "case-1"), registryBaseAt.Add(time.Hour))
	if err != nil || len(items) != 2 {
		t.Fatalf("盘点：err=%v items=%d", err, len(items))
	}
	// ORDER BY obligation：BROKER/FILING 在前。
	if items[0].State != domain.ObligationHandedOver || items[0].HandedTo != "BROKER/ACME" {
		t.Fatalf("承接项走样：%+v", items[0])
	}
	if items[1].State != domain.ObligationConcluded || items[1].HandedTo != "" {
		t.Fatalf("非承接项带回了承接对象：%+v", items[1])
	}
}

// 先登目录再登明细的顺序由库的外键守着，不靠调用方记得。
func TestAnObligationItemWithoutItsCatalogIsRejected(t *testing.T) {
	registry, _, fixture := newObligationRegistry(t)
	registration := ports.ObligationRegistration{
		Item: domain.ClosureObligationItem{
			Obligation: "DUTY/PAYMENT",
			Scope:      "case-1",
			State:      domain.ObligationUnresolved,
			Basis:      "PROGRAM/DDP",
		},
		AppliesFrom: registryBaseAt,
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterObligationItem(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"),
			viewValue(t, domain.NewCustomsCaseID, "case-1"), registration)
	}); err == nil {
		t.Fatal("目录不在时明细仍被写了进去")
	}
}

// 没有适用区间起点的义务项一次截点都盘不进来，等于登记了却永远不参与关闭判断。
// 写口在进库前就挡下它，而不是让它成为一条永远盘不出的死行。
func TestAnObligationWithoutAnIntervalStartIsRejected(t *testing.T) {
	registry, _, fixture := newObligationRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	registerObligationCatalog(t, registry, fixture, tenant, "case-1")

	registration := ports.ObligationRegistration{
		Item: domain.ClosureObligationItem{
			Obligation: "DUTY/PAYMENT",
			Scope:      "case-1",
			State:      domain.ObligationUnresolved,
			Basis:      "PROGRAM/DDP",
		},
	}
	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterObligationItem(ctx, tenant,
			viewValue(t, domain.NewCustomsCaseID, "case-1"), registration)
	})
	if err == nil {
		t.Fatal("没有区间起点的义务项被收下了")
	}
	if outcome != ports.CaseConfigurationSaveOutcomeInvalid {
		t.Fatalf("该交回`非法`，得到 %v", outcome)
	}
}

func newGateRegistry(t *testing.T) (*adapter.GateConditionRegistrations, *adapter.GateConditionView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewGateConditionRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造门禁写口：%v", err)
	}
	view, err := adapter.NewGateConditionView(fixture.db)
	if err != nil {
		t.Fatalf("构造门禁读口：%v", err)
	}
	return registry, view, fixture
}

// 门禁这一侧的空清单格比义务那侧更要紧：登记了目录却无前置条件，领域折成`不适用`
// （此动作在此边界本就不受门禁）。若登记方表达不出它，「不受管」就只能靠「查不到」
// 冒充，而「查不到」本该是未决——等于把门禁放开。
func TestRegisteringOnlyTheGateCatalogYieldsAnEmptyFindingListRatherThanUnconfigured(t *testing.T) {
	registry, view, fixture := newGateRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	scope := viewValue(t, domain.NewDecisionScopeReference, "case-1/unit-1")
	boundary := viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, boundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}

	findings, configured, err := view.LoadPreconditionFindings(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || !configured {
		t.Fatalf("登了目录却答未配置：err=%v configured=%v", err, configured)
	}
	if len(findings) != 0 {
		t.Fatalf("空清单里冒出了 %d 项", len(findings))
	}
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GateNotApplicable {
		t.Fatalf("空清单没折成`不适用`：err=%v conclusion=%v", err, conclusion)
	}
}

func TestGateFindingsRoundTripThroughTheView(t *testing.T) {
	registry, view, fixture := newGateRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	scope := viewValue(t, domain.NewDecisionScopeReference, "case-1/unit-1")
	boundary := viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, boundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}
	for _, finding := range []domain.PreconditionFinding{
		{Precondition: viewValue(t, domain.NewPreconditionReference, "DECLARATION/ACCEPTED"), State: domain.PreconditionMet},
		{Precondition: viewValue(t, domain.NewPreconditionReference, "DUTY/SETTLED"), State: domain.PreconditionUnmet},
	} {
		if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return registry.RegisterGateFinding(ctx, tenant, scope, domain.OutboundRelease, boundary, finding)
		}); err != nil {
			t.Fatalf("登记前置条件 %s：%v", finding.Precondition, err)
		}
	}

	findings, configured, err := view.LoadPreconditionFindings(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || !configured || len(findings) != 2 {
		t.Fatalf("门禁往返走样：err=%v configured=%v findings=%d", err, configured, len(findings))
	}
	conclusion, err := domain.FoldGateConclusion(findings)
	if err != nil || conclusion != domain.GatePartiallyMet {
		t.Fatalf("一满足一未满足该折成`部分满足`：err=%v conclusion=%v", err, conclusion)
	}
}

// 门禁判断绑定动作与边界，不得复用于其他动作（CONTEXT「不能复用于其他动作或监管边界」）：换个动作是另一本
// 目录，未登记即未决。
func TestAGateCatalogDoesNotAnswerForAnotherAction(t *testing.T) {
	registry, view, fixture := newGateRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	scope := viewValue(t, domain.NewDecisionScopeReference, "case-1/unit-1")
	boundary := viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, boundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}
	if _, configured, err := view.LoadPreconditionFindings(t.Context(), tenant, scope,
		domain.LoadingDeparture, boundary); err != nil || configured {
		t.Fatalf("出境放行的目录替装载起运作了答：err=%v configured=%v", err, configured)
	}
}

// 前置条件目录不在时，逐项判断进不去——同义务那侧，顺序由外键守着。
func TestAGateFindingWithoutItsCatalogIsRejected(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateFinding(ctx,
			viewValue(t, domain.NewTenantID, "tenant-a"),
			viewValue(t, domain.NewDecisionScopeReference, "case-1/unit-1"),
			domain.OutboundRelease,
			viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL"),
			domain.PreconditionFinding{
				Precondition: viewValue(t, domain.NewPreconditionReference, "DUTY/SETTLED"),
				State:        domain.PreconditionMet,
			})
	}); err == nil {
		t.Fatal("目录不在时逐项判断仍被写了进去")
	}
}

// 封闭取值之外的动作与状态不静默进库：多出来的取值会让读口在译回时报错，而那时
// 现场已经离写入很远了。
func TestUnknownGateEnumerationsAreRejectedAtTheRegistrationPort(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	scope := viewValue(t, domain.NewDecisionScopeReference, "case-1/unit-1")
	boundary := viewValue(t, domain.NewCustomsProcedureReference, "EXPORT/GENERAL")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.GuardedActionInvalid, boundary, registryBaseAt)
	}); err == nil {
		t.Fatal("非法受管动作被静默登记")
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, boundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateFinding(ctx, tenant, scope, domain.OutboundRelease, boundary,
			domain.PreconditionFinding{
				Precondition: viewValue(t, domain.NewPreconditionReference, "DUTY/SETTLED"),
				State:        domain.PreconditionStateInvalid,
			})
	}); err == nil {
		t.Fatal("非法前置条件状态被静默登记")
	}
}
