package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 税费付款协作、外部资金事实引用与税费付款核对三张表的往返用例（0016 建表，票
// mechanism-executor-triage/07 CC-c）。做法承 case_config_registry_test.go 头注那两条：断言
// 穿读口取回、写入一律进环境事务。三张表的约束与 SQL 是新写的，逐格钉一遍。

var dutyRegistryBaseAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

func newDutyReconciliation(t *testing.T) (*adapter.DutyPaymentReconciliation, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	store, err := adapter.NewDutyPaymentReconciliation(fixture.db)
	if err != nil {
		t.Fatalf("构造税费付款核对登记册：%v", err)
	}
	return store, fixture
}

func synCollaboration(t *testing.T, kind domain.DutyObligationKind, target string) domain.DutyPaymentCollaboration {
	t.Helper()
	spec := domain.DutyCollaborationSpec{
		Kind:        kind,
		Scope:       viewValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Obligor:     viewValue(t, domain.NewLegalObligorReference, "SYN-OBLIGOR-01"),
		Requirement: viewValue(t, domain.NewPaymentRequirementSource, "SYN-ASSESSMENT-01"),
		Target:      viewValue(t, domain.NewResponsibilityTargetReference, target),
		FormedAt:    dutyRegistryBaseAt,
	}
	switch kind {
	case domain.ObligationFromAssessedDuty:
		spec.Duty = viewValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	case domain.ObligationExplicitlyNotRequired:
		spec.NoPayBasis = "SYN-PROGRAM-01: no duty on this scope"
	}
	collaboration, err := domain.FormDutyCollaboration(spec)
	if err != nil {
		t.Fatalf("构造协作事项：%v", err)
	}
	return collaboration
}

func synFundsFact(t *testing.T, amount int64) ports.ExternalFundsFactRegistration {
	t.Helper()
	return ports.ExternalFundsFactRegistration{
		Fact:        viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
		Version:     viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
		Source:      "SYN-BANK-01",
		Payer:       viewValue(t, domain.ProvidedFundsPayer, "SYN-PAYER-01"),
		Currency:    "XTS",
		AmountMinor: amount,
		OccurredAt:  dutyRegistryBaseAt.Add(-time.Hour),
	}
}

func synProcedure(t *testing.T, value string) domain.CustomsProcedureReference {
	t.Helper()
	return viewValue(t, domain.NewCustomsProcedureReference, value)
}

func synVerificationRecord(t *testing.T, coverage domain.DutyCoverage, digest string) ports.DutyVerificationRecord {
	t.Helper()
	duty := viewValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	funds := viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01")
	fundsVersion := viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1")
	scope := viewValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01")
	verification, err := domain.VerifyDutyPayment(duty, funds, fundsVersion, scope, synProcedure(t, "SYN-PROC-IMPORT"),
		coverage, domain.DeltaNone, domain.FundsFactValid, dutyRegistryBaseAt)
	if err != nil {
		t.Fatalf("构造核对：%v", err)
	}
	return ports.DutyVerificationRecord{
		Key: ports.DutyVerificationKey{
			TenantID: viewValue(t, domain.NewTenantID, "tenant-a"),
			Duty:     duty, Funds: funds, Scope: scope, Digest: digest,
		},
		Verification: verification,
		Basis:        "SYN-RULE-01: assessment reference quoted on the remittance",
	}
}

func tenantA(t *testing.T) domain.TenantID {
	t.Helper()
	return viewValue(t, domain.NewTenantID, "tenant-a")
}

// Covers: 协作事项两格各自往返——核定税费格的税费引用与无需付款格的依据逐格如实读回；两格
// 在同一范围上各占一行，互不顶替。
func TestBothCollaborationKindsRoundTrip(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	assessed := synCollaboration(t, domain.ObligationFromAssessedDuty, "SYN-DUTY-DESK")
	notRequired := synCollaboration(t, domain.ObligationExplicitlyNotRequired, "SYN-DUTY-DESK")

	for _, collaboration := range []domain.DutyPaymentCollaboration{assessed, notRequired} {
		outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.SaveCollaboration(ctx, tenantA(t), collaboration)
		})
		if err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("登记 %s：err=%v outcome=%v", collaboration.Kind(), err, outcome)
		}
	}

	duty, _ := assessed.Duty()
	loadedAssessed, found, err := store.FindCollaboration(t.Context(), tenantA(t), assessed.Scope(), duty)
	if err != nil || !found {
		t.Fatalf("核定格读不回：err=%v found=%v", err, found)
	}
	loadedDuty, hasDuty := loadedAssessed.Duty()
	if loadedAssessed.Kind() != domain.ObligationFromAssessedDuty || !hasDuty || loadedDuty != duty ||
		loadedAssessed.Obligor() != assessed.Obligor() || loadedAssessed.Requirement() != assessed.Requirement() ||
		loadedAssessed.Target() != assessed.Target() || !loadedAssessed.FormedAt().Equal(dutyRegistryBaseAt) {
		t.Fatalf("核定格走样：%+v", loadedAssessed)
	}

	loadedNotRequired, found, err := store.FindCollaboration(t.Context(), tenantA(t), notRequired.Scope(), domain.AssessedDutyReference{})
	if err != nil || !found {
		t.Fatalf("无需付款格读不回：err=%v found=%v", err, found)
	}
	basis, hasBasis := loadedNotRequired.NoPayBasis()
	if loadedNotRequired.Kind() != domain.ObligationExplicitlyNotRequired || !hasBasis ||
		basis != "SYN-PROGRAM-01: no duty on this scope" {
		t.Fatalf("无需付款格走样：%+v", loadedNotRequired)
	}
}

// Covers: 同（范围，税费引用）重登交回`已登记`且不顶替——换责任交接目标也一样，库里仍是首版。
func TestSameCollaborationKeyNeverReplacesTheFirstVersion(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	first := synCollaboration(t, domain.ObligationFromAssessedDuty, "SYN-DUTY-DESK")
	second := synCollaboration(t, domain.ObligationFromAssessedDuty, "SYN-OTHER-DESK")

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.SaveCollaboration(ctx, tenantA(t), first)
	}); err != nil {
		t.Fatalf("首登：%v", err)
	}
	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.SaveCollaboration(ctx, tenantA(t), second)
	})
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登该交回`已登记`：err=%v outcome=%v", err, outcome)
	}
	duty, _ := first.Duty()
	loaded, _, err := store.FindCollaboration(t.Context(), tenantA(t), first.Scope(), duty)
	if err != nil || loaded.Target().String() != "SYN-DUTY-DESK" {
		t.Fatalf("首版被顶替：err=%v target=%v", err, loaded.Target())
	}
}

// Covers: 资金事实引用往返——各维如实读回；同（引用 + 版本）重登`已登记`不顶替；未登记 found=false（按版本点读，
// 事实在册而版本不在也是 found=false——票 sa-cc/19 起核对按版本读前置）。
func TestExternalFundsFactsRoundTripByReference(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	registration := synFundsFact(t, 12500)

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), registration)
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact, registration.Version)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	if loaded.Fact != registration.Fact || loaded.Source != "SYN-BANK-01" || loaded.Payer != registration.Payer ||
		loaded.Currency != "XTS" || loaded.AmountMinor != 12500 || !loaded.OccurredAt.Equal(registration.OccurredAt) {
		t.Fatalf("资金事实行走样：%+v", loaded)
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 99))
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同引用重登该交回`已登记`：err=%v outcome=%v", err, replay)
	}
	if again, _, _ := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact, registration.Version); again.AmountMinor != 12500 {
		t.Fatalf("首版金额被顶替：%d", again.AmountMinor)
	}
	if _, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t),
		viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-NOBODY"), registration.Version); err != nil || found {
		t.Fatalf("未登记该 found=false：err=%v found=%v", err, found)
	}
	if _, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact,
		viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v9")); err != nil || found {
		t.Fatalf("事实在册而版本不在该 found=false：err=%v found=%v", err, found)
	}
	if _, _, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact, domain.FundsFactVersion{}); err == nil {
		t.Fatal("空白版本被当成正常查询")
	}
	if versions, err := store.ListFundsFactVersions(t.Context(), tenantA(t),
		viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-NOBODY")); err != nil || len(versions) != 0 {
		t.Fatalf("未登记该列空：err=%v n=%d", err, len(versions))
	}
}

// Covers: 票 sa-cc/13 完成判据 2「新迁移往返」——同一事实的更正版本 v2（回指 v1、金额变）落版本子表第二行，
// 身份行仍只一行；ListFundsFactVersions 按接收先后列两版、v2 回指 v1、v1 一字不动；两版各自按版本点读得回；
// 同版本重登`已登记`不顶替。直读库面：身份表 0021 起只剩身份列，内容与版本都在子表。
//
// 登记顺序显式先 v1 后 v2：两版各自一笔事务、received_at 各取事务时钟，登记顺序就是接收顺序，「按接收先后列」
// 的断言只在这个顺序确定时成立——交给 map 迭代一类的随机源，断言会随机翻面（sa-cc/26）。反着到的那一格在
// TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadTheLatestReceived 里另钉。
func TestFundsFactVersionsAccrueAsRowsThatPointBack(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	first := synFundsFact(t, 12500)
	second := synFundsFact(t, 9000)
	second.Version = viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v2")
	second.Corrects = first.Version

	for _, step := range []struct {
		name         string
		registration ports.ExternalFundsFactRegistration
	}{{"v1", first}, {"v2", second}} {
		outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.RegisterFundsFact(ctx, tenantA(t), step.registration)
		})
		if err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("%s：err=%v outcome=%v", step.name, err, outcome)
		}
	}
	versions, err := store.ListFundsFactVersions(t.Context(), tenantA(t), first.Fact)
	if err != nil || len(versions) != 2 {
		t.Fatalf("该列两版：err=%v n=%d", err, len(versions))
	}
	if versions[0].Version != first.Version || versions[0].Corrects != (domain.FundsFactVersion{}) || versions[0].AmountMinor != 12500 {
		t.Fatalf("v1 走样：%+v", versions[0])
	}
	if versions[1].Version != second.Version || versions[1].Corrects != first.Version || versions[1].AmountMinor != 9000 ||
		versions[1].Payer != second.Payer || !versions[1].OccurredAt.Equal(second.OccurredAt) {
		t.Fatalf("v2 走样：%+v", versions[1])
	}
	for _, want := range []ports.ExternalFundsFactRegistration{first, second} {
		got, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), first.Fact, want.Version)
		if err != nil || !found || got != want {
			t.Fatalf("按版本点读 %s 该原样交回：err=%v found=%v got=%+v", want.Version, err, found, got)
		}
	}

	var identityRows, versionRows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM customs_compliance.external_funds_fact WHERE tenant_id = $1 AND fact_ref = $2),
		        (SELECT count(*) FROM customs_compliance.external_funds_fact_version WHERE tenant_id = $1 AND fact_ref = $2)`,
		"tenant-a", first.Fact.String()).Scan(&identityRows, &versionRows); err != nil {
		t.Fatalf("直读行数：%v", err)
	}
	if identityRows != 1 || versionRows != 2 {
		t.Fatalf("身份行 %d / 版本行 %d，要 1 / 2", identityRows, versionRows)
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		changed := second
		changed.AmountMinor = 1
		return store.RegisterFundsFact(ctx, tenantA(t), changed)
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同版本重登该`已登记`：err=%v outcome=%v", err, replay)
	}
	if again, _ := store.ListFundsFactVersions(t.Context(), tenantA(t), first.Fact); again[1].AmountMinor != 9000 {
		t.Fatalf("同版本重登顶替了内容：%d", again[1].AmountMinor)
	}
}

// Covers: 同一事实的两版反着到——先到 v2（回指 v1）后到 v1——各占一行（UC-CC-009「不按最后到达覆盖」、票 sa-cc/13
// 裁决 1「迟到的前版按自己的版本进」）；ListFundsFactVersions 按接收先后列为 [v2, v1]，如实反映到达顺序、不按版本
// 字面重排；按版本点读各得各的——到达顺序不决定谁被读到（票 sa-cc/19 起「最近接收」读口退役，26 那一格的最后一条
// 断言随之改口）。它与 TestFundsFactVersionsAccrueAsRowsThatPointBack 是同一条规则的两个到达顺序，各钉一格，顺序
// 都在用例里显式写死。
func TestFundsFactVersionsArrivingOutOfOrderListByReceiptAndLoadByVersion(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	late := synFundsFact(t, 500)
	late.Fact = viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-02")
	late.Version = viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-02/v2")
	late.Corrects = viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-02/v1")
	earlier := synFundsFact(t, 400)
	earlier.Fact = late.Fact
	earlier.Version = late.Corrects

	for index, registration := range []ports.ExternalFundsFactRegistration{late, earlier} {
		if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.RegisterFundsFact(ctx, tenantA(t), registration)
		}); err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("乱序第 %d 版：err=%v outcome=%v", index+1, err, outcome)
		}
	}
	versions, err := store.ListFundsFactVersions(t.Context(), tenantA(t), late.Fact)
	if err != nil || len(versions) != 2 {
		t.Fatalf("该列两版：err=%v n=%d", err, len(versions))
	}
	if versions[0].Version != late.Version || versions[0].Corrects != earlier.Version || versions[0].AmountMinor != 500 {
		t.Fatalf("先到的 v2 该列在前、回指 v1：%+v", versions[0])
	}
	if versions[1].Version != earlier.Version || versions[1].Corrects != (domain.FundsFactVersion{}) || versions[1].AmountMinor != 400 {
		t.Fatalf("迟到的前版 v1 该按自己的版本进、列在后：%+v", versions[1])
	}
	for _, want := range []ports.ExternalFundsFactRegistration{late, earlier} {
		got, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), late.Fact, want.Version)
		if err != nil || !found || got != want {
			t.Fatalf("按版本点读 %s 不受到达顺序影响：err=%v found=%v got=%+v", want.Version, err, found, got)
		}
	}
}

// Covers: 票 sa-cc/12 完成判据 2「放宽后的往返」——付款人「来源未提供」落成 payer_ref 为 NULL、读回仍是那一格
// （不是空串、不是零值）；同引用重登`已登记`不顶替；「提供了」的行照旧读回引用本身。
func TestAFundsFactWithoutAPayerRoundTripsAsExplicitlyNotProvided(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	registration := synFundsFact(t, 12500)
	registration.Payer = domain.FundsPayerNotProvided()

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), registration)
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("来源未提供付款人的事实该登得进：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact, registration.Version)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	if loaded.Payer.Provided() || !loaded.Payer.Valid() || loaded.Payer != domain.FundsPayerNotProvided() {
		t.Fatalf("付款人该读回「来源未提供」那一格，实得 %#v", loaded.Payer)
	}
	if loaded.Source != "SYN-BANK-01" || loaded.Currency != "XTS" || loaded.AmountMinor != 12500 {
		t.Fatalf("其余维度走样：%+v", loaded)
	}

	var payerColumn *string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT payer_ref FROM customs_compliance.external_funds_fact_version WHERE tenant_id = $1 AND fact_ref = $2 AND version = $3`,
		"tenant-a", registration.Fact.String(), registration.Version.String()).Scan(&payerColumn); err != nil {
		t.Fatalf("直读 payer_ref：%v", err)
	}
	if payerColumn != nil {
		t.Fatalf("「来源未提供」在库上该是 NULL，实得 %q", *payerColumn)
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 12500))
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同引用重登该交回`已登记`：err=%v outcome=%v", err, replay)
	}
	if again, _, _ := store.LoadFundsFactVersion(t.Context(), tenantA(t), registration.Fact, registration.Version); again.Payer.Provided() {
		t.Fatal("首版「未提供」被「提供了」顶替")
	}

	zero := synFundsFact(t, 1)
	zero.Fact = viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-02")
	zero.Payer = domain.FundsPayer{}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), zero)
	}); err == nil {
		t.Fatal("零值付款人被登进去了——两格都不是不该落库")
	}
}

// Covers: 票 sa-cc/12 裁决 1——「真实程序要不要求付款人」按（租户、监管程序）一行一条：两形各自往返、同键重登
// `已登记`不顶替（改规则走复核另登）、未登记 found=false（核对编排据此答「规则未配置」，不取默认）。
func TestPayerRequirementRulesRoundTripPerProcedure(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	requiring := synProcedure(t, "SYN-PROC-REQUIRES-PAYER")
	waiving := synProcedure(t, "SYN-PROC-NO-PAYER")

	for procedure, requirement := range map[domain.CustomsProcedureReference]domain.PayerRequirement{
		requiring: domain.PayerRequired,
		waiving:   domain.PayerNotRequired,
	} {
		outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.RegisterPayerRequirement(ctx, tenantA(t), procedure, requirement)
		})
		if err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("登记 %s=%s：err=%v outcome=%v", procedure, requirement, err, outcome)
		}
		loaded, found, err := store.LoadPayerRequirement(t.Context(), tenantA(t), procedure)
		if err != nil || !found || loaded != requirement {
			t.Fatalf("%s 读回 = %v found=%v err=%v, want %v", procedure, loaded, found, err, requirement)
		}
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterPayerRequirement(ctx, tenantA(t), requiring, domain.PayerNotRequired)
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登该交回`已登记`：err=%v outcome=%v", err, replay)
	}
	if again, _, _ := store.LoadPayerRequirement(t.Context(), tenantA(t), requiring); again != domain.PayerRequired {
		t.Fatalf("首登被顶替：%v", again)
	}

	if _, found, err := store.LoadPayerRequirement(t.Context(), tenantA(t), synProcedure(t, "SYN-PROC-UNREGISTERED")); err != nil || found {
		t.Fatalf("未登记该 found=false：err=%v found=%v", err, found)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterPayerRequirement(ctx, tenantA(t), synProcedure(t, "SYN-PROC-ZERO"), domain.PayerRequirementInvalid)
	}); err == nil {
		t.Fatal("零值规则被登进去了")
	}
	if _, err := store.RegisterPayerRequirement(t.Context(), tenantA(t), requiring, domain.PayerRequired); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记规则应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 核对往返——三轴、依据、验证时间如实读回；同键重放`已登记`；换指纹（改判）是另一行，
// 两版都在。
func TestVerificationsRoundTripAndVersionsAccrue(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 12500))
	}); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	first := synVerificationRecord(t, domain.CoverageFull, "digest-v1")

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.SaveVerification(ctx, first)
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首版：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := store.FindVerification(t.Context(), first.Key)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	if loaded.Verification.Coverage() != domain.CoverageFull || loaded.Verification.Delta() != domain.DeltaNone ||
		loaded.Verification.Validity() != domain.FundsFactValid || loaded.Basis != first.Basis ||
		loaded.Verification.Duty() != first.Verification.Duty() || loaded.Verification.Funds() != first.Verification.Funds() ||
		loaded.Verification.Scope() != first.Verification.Scope() ||
		loaded.Verification.Procedure() != first.Verification.Procedure() ||
		!loaded.Verification.VerifiedAt().Equal(dutyRegistryBaseAt) {
		t.Fatalf("核对走样：%+v", loaded)
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.SaveVerification(ctx, first)
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重放该交回`已登记`：err=%v outcome=%v", err, replay)
	}
	second := synVerificationRecord(t, domain.CoverageNone, "digest-v2")
	appended, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.SaveVerification(ctx, second)
	})
	if err != nil || appended != ports.CaseConfigurationRegistered {
		t.Fatalf("改判该是新版本：err=%v outcome=%v", err, appended)
	}
	if v1, found, _ := store.FindVerification(t.Context(), first.Key); !found || v1.Verification.Coverage() != domain.CoverageFull {
		t.Fatalf("新版本覆盖了前版：found=%v", found)
	}
}

// Covers: 票 sa-cc/22 完成判据 (2)——核对记录带程序、真库往返：`0022` 加的 `procedure_ref` 经点读、当前一版、上列
// 三条读路各自读回同一个程序（裁决 1 点名的四处读回里库侧的三处；HTTP JSON 在 adapters/http 自己钉）；同键
// 不同程序的两行并存、各自带着自己的程序（程序进指纹不进主键，裁决 2——这里指纹由用例给，两行只靠指纹分开）；
// 库内 CHECK 把空白程序挡在门外（旁路写入用显式 SQL）。
func TestVerificationsRoundTripTheProcedureTheyWereJudgedUnder(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	catalogue, err := adapter.NewDutyReconciliationCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造核对目录：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 12500))
	}); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	underImport := synVerificationRecord(t, domain.CoverageFull, "digest-under-import")
	underExport := synVerificationRecord(t, domain.CoverageFull, "digest-under-export")
	exportVerification, err := domain.VerifyDutyPayment(
		underExport.Key.Duty, underExport.Key.Funds, underExport.Verification.FundsVersion(), underExport.Key.Scope,
		synProcedure(t, "SYN-PROC-EXPORT"),
		domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, dutyRegistryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("构造按出口程序判的核对：%v", err)
	}
	underExport.Verification = exportVerification
	for _, record := range []ports.DutyVerificationRecord{underImport, underExport} {
		if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.SaveVerification(ctx, record)
		}); err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("登记 %s：err=%v outcome=%v", record.Key.Digest, err, outcome)
		}
	}

	for _, record := range []ports.DutyVerificationRecord{underImport, underExport} {
		loaded, found, err := store.FindVerification(t.Context(), record.Key)
		if err != nil || !found || loaded.Verification.Procedure() != record.Verification.Procedure() {
			t.Fatalf("点读 %s 该带回它按哪个程序判：err=%v found=%v procedure=%q",
				record.Key.Digest, err, found, loaded.Verification.Procedure())
		}
	}
	current, found, err := store.LoadCurrentDutyVerification(t.Context(), tenantA(t), underExport.Key.Scope)
	if err != nil || !found || current.Key.Digest != "digest-under-export" ||
		current.Verification.Procedure() != underExport.Verification.Procedure() {
		t.Fatalf("当前一版该是核对时刻更晚的出口那版并带程序：err=%v found=%v %+v", err, found, current)
	}
	listed, err := catalogue.ListDutyVerifications(t.Context(), tenantA(t), 10)
	if err != nil || len(listed) != 2 {
		t.Fatalf("上列该两版：err=%v n=%d", err, len(listed))
	}
	byDigest := map[string]domain.CustomsProcedureReference{}
	for _, record := range listed {
		byDigest[record.Key.Digest] = record.Verification.Procedure()
	}
	if byDigest["digest-under-import"] != underImport.Verification.Procedure() ||
		byDigest["digest-under-export"] != underExport.Verification.Procedure() {
		t.Fatalf("上列没有逐行带回各自的程序：%v", byDigest)
	}

	verification := `INSERT INTO customs_compliance.duty_payment_verification
		(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, funds_version, procedure_ref, coverage, delta, validity, basis, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	fixture.rejects(t, "程序空白", verification,
		"tenant-a", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "SYN-UNIT-01", "digest-blank-procedure", "SYN-FUNDS-01/v1", "  ",
		"COVERED", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
}

// Covers: 票 sa-cc/19 完成判据 (2)「0023 往返」——核对记录带「比的是资金事实的哪一版」：写口落 `funds_version`、点读
// 读回同一版；ListVerificationsByFundsFact 按（租户、资金事实）列全部核对版本、核对时刻升序、同一时刻按指纹字典序，
// 别的事实的核对与别的租户不可见、没核对过的事实答空；库内 0023 的外键把「引用没接收过的那一版」挡在门外、CHECK 拒
// 空白版本（旁路写入用显式 SQL）。
func TestVerificationsRoundTripTheFundsFactVersionTheyJudgedAndListByFundsFact(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	first := synFundsFact(t, 12500)
	second := synFundsFact(t, 9000)
	second.Version = viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v2")
	second.Corrects = first.Version
	other := synFundsFact(t, 1)
	other.Fact = viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-02")
	other.Version = viewValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-02/v1")
	for _, registration := range []ports.ExternalFundsFactRegistration{first, second, other} {
		if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.RegisterFundsFact(ctx, tenantA(t), registration)
		}); err != nil {
			t.Fatalf("资金事实 %s：%v", registration.Version, err)
		}
	}

	onFirst := synVerificationRecord(t, domain.CoverageFull, "digest-on-v1")
	onSecond := synVerificationRecord(t, domain.CoverageFull, "digest-on-v2")
	onSecondVerification, err := domain.VerifyDutyPayment(
		onSecond.Key.Duty, onSecond.Key.Funds, second.Version, onSecond.Key.Scope, synProcedure(t, "SYN-PROC-IMPORT"),
		domain.CoverageFull, domain.DeltaPending, domain.FundsFactPending, dutyRegistryBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("构造按 v2 判的核对：%v", err)
	}
	onSecond.Verification = onSecondVerification
	sameInstant := synVerificationRecord(t, domain.CoverageNone, "digest-aaa-same-instant")
	onOther := synVerificationRecord(t, domain.CoverageFull, "digest-on-other")
	onOther.Key.Funds = other.Fact
	onOtherVerification, err := domain.VerifyDutyPayment(
		onOther.Key.Duty, other.Fact, other.Version, onOther.Key.Scope, synProcedure(t, "SYN-PROC-IMPORT"),
		domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, dutyRegistryBaseAt)
	if err != nil {
		t.Fatalf("构造另一条事实的核对：%v", err)
	}
	onOther.Verification = onOtherVerification
	for _, record := range []ports.DutyVerificationRecord{onSecond, onFirst, sameInstant, onOther} {
		if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.SaveVerification(ctx, record)
		}); err != nil || outcome != ports.CaseConfigurationRegistered {
			t.Fatalf("登记 %s：err=%v outcome=%v", record.Key.Digest, err, outcome)
		}
	}

	loaded, found, err := store.FindVerification(t.Context(), onSecond.Key)
	if err != nil || !found || loaded.Verification.FundsVersion() != second.Version {
		t.Fatalf("点读该带回比的是哪一版：err=%v found=%v version=%q", err, found, loaded.Verification.FundsVersion())
	}
	listed, err := store.ListVerificationsByFundsFact(t.Context(), tenantA(t), first.Fact)
	if err != nil || len(listed) != 3 {
		t.Fatalf("SYN-FUNDS-01 该列三版（另一条事实的不算）：err=%v n=%d", err, len(listed))
	}
	if listed[0].Key.Digest != "digest-aaa-same-instant" || listed[1].Key.Digest != "digest-on-v1" || listed[2].Key.Digest != "digest-on-v2" {
		t.Fatalf("该按核对时刻升序、同一时刻按指纹字典序：%q / %q / %q", listed[0].Key.Digest, listed[1].Key.Digest, listed[2].Key.Digest)
	}
	if listed[1].Verification.FundsVersion() != first.Version || listed[2].Verification.FundsVersion() != second.Version ||
		listed[2].Verification.Delta() != domain.DeltaPending || listed[2].Basis != onSecond.Basis {
		t.Fatalf("上列每行该带自己的资金版本与内容：%+v", listed)
	}
	if none, err := store.ListVerificationsByFundsFact(t.Context(), tenantA(t),
		viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-NOBODY")); err != nil || len(none) != 0 {
		t.Fatalf("没核对过的事实该答空：err=%v n=%d", err, len(none))
	}
	if _, err := store.ListVerificationsByFundsFact(t.Context(), tenantA(t), domain.ExternalFundsFactReference{}); err == nil {
		t.Fatal("空事实引用被当成正常查询")
	}

	verification := `INSERT INTO customs_compliance.duty_payment_verification
		(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, funds_version, procedure_ref, coverage, delta, validity, basis, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	fixture.rejects(t, "引用没接收过的那一版", verification,
		"tenant-a", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "SYN-UNIT-01", "digest-on-v9", "SYN-FUNDS-01/v9", "SYN-PROC-IMPORT",
		"COVERED", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
	fixture.rejects(t, "资金版本空白", verification,
		"tenant-a", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "SYN-UNIT-01", "digest-blank-version", "  ", "SYN-PROC-IMPORT",
		"COVERED", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
}

// Covers: 库内再守一遍形状——协作两格的矛盾形状、种类集外、负金额、无依据的核对、没有资金
// 事实的核对（外键）、三轴集外都被挡在门外。旁路写入用显式 SQL。
func TestTheReconciliationTablesRejectWhatTheDomainRejects(t *testing.T) {
	_, fixture := newDutyReconciliation(t)

	collaboration := `INSERT INTO customs_compliance.duty_payment_collaboration
		(tenant_id, scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	fixture.rejects(t, "核定格带无需付款依据", collaboration,
		"tenant-a", "SYN-UNIT-X", "SYN-DUTY-X", "ASSESSED_DUTY", "basis", "o", "r", "t", dutyRegistryBaseAt)
	fixture.rejects(t, "无需付款格带税费引用", collaboration,
		"tenant-a", "SYN-UNIT-X", "SYN-DUTY-X", "EXPLICITLY_NOT_REQUIRED", "basis", "o", "r", "t", dutyRegistryBaseAt)
	fixture.rejects(t, "无需付款格无依据", collaboration,
		"tenant-a", "SYN-UNIT-X", "", "EXPLICITLY_NOT_REQUIRED", "  ", "o", "r", "t", dutyRegistryBaseAt)
	fixture.rejects(t, "种类集外", collaboration,
		"tenant-a", "SYN-UNIT-X", "SYN-DUTY-X", "MAYBE", "", "o", "r", "t", dutyRegistryBaseAt)

	// 0021 起内容在版本子表；身份行先落，子表外键钉「版本属于某条已登记的事实」。
	identity := `INSERT INTO customs_compliance.external_funds_fact (tenant_id, fact_ref, received_at) VALUES ($1, $2, $3)`
	fixture.seed(t, identity, "tenant-a", "SYN-FUNDS-X", dutyRegistryBaseAt)
	funds := `INSERT INTO customs_compliance.external_funds_fact_version
		(tenant_id, fact_ref, version, corrects_version, source_ref, payer_ref, currency, amount_minor, occurred_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	fixture.rejects(t, "负金额", funds,
		"tenant-a", "SYN-FUNDS-X", "v1", nil, "SYN-BANK-01", "SYN-PAYER-01", "XTS", -1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "币种空白", funds,
		"tenant-a", "SYN-FUNDS-X", "v1", nil, "SYN-BANK-01", "SYN-PAYER-01", " ", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	// 0020 起：付款人可以是 NULL（来源未提供），但空白串仍不是任何一格。
	fixture.rejects(t, "付款人空白串", funds,
		"tenant-a", "SYN-FUNDS-X", "v1", nil, "SYN-BANK-01", "  ", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "版本空白", funds,
		"tenant-a", "SYN-FUNDS-X", "  ", nil, "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "回指自己", funds,
		"tenant-a", "SYN-FUNDS-X", "v1", "v1", "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "回指空白", funds,
		"tenant-a", "SYN-FUNDS-X", "v1", "  ", "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "版本不属于已登记的事实", funds,
		"tenant-a", "SYN-FUNDS-NOBODY", "v1", nil, "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)

	payerRule := `INSERT INTO customs_compliance.duty_payment_payer_rule
		(tenant_id, procedure_ref, payer_requirement, registered_at)
		VALUES ($1, $2, $3, $4)`
	fixture.rejects(t, "付款人规则词形集外", payerRule, "tenant-a", "SYN-PROC-X", "MAYBE", dutyRegistryBaseAt)
	fixture.rejects(t, "付款人规则程序空白", payerRule, "tenant-a", "  ", "REQUIRED", dutyRegistryBaseAt)

	verification := `INSERT INTO customs_compliance.duty_payment_verification
		(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, funds_version, procedure_ref, coverage, delta, validity, basis, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	fixture.rejects(t, "没有资金事实的核对", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-NOBODY", "SYN-UNIT-X", "d", "v1", "SYN-PROC-X", "COVERED", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
	fixture.seed(t, funds,
		"tenant-a", "SYN-FUNDS-X", "v1", nil, "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "无依据的核对", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "v1", "SYN-PROC-X", "COVERED", "NO_DELTA", "VALID", "  ", dutyRegistryBaseAt)
	fixture.rejects(t, "覆盖轴集外", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "v1", "SYN-PROC-X", "MAYBE", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
	fixture.rejects(t, "有效性轴集外", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "v1", "SYN-PROC-X", "COVERED", "NO_DELTA", "MAYBE", "basis", dutyRegistryBaseAt)
}

// Covers: 三个写方法在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired）。
func TestReconciliationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _ := newDutyReconciliation(t)
	ctx := t.Context()

	if _, err := store.SaveCollaboration(ctx, tenantA(t),
		synCollaboration(t, domain.ObligationFromAssessedDuty, "SYN-DUTY-DESK")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记协作事项应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 1)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记资金事实应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := store.SaveVerification(ctx,
		synVerificationRecord(t, domain.CoverageFull, "d")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记核对应返回 ErrTransactionRequired，实得：%v", err)
	}
}
