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
		Source:      "SYN-BANK-01",
		Payer:       "SYN-PAYER-01",
		Currency:    "XTS",
		AmountMinor: amount,
		OccurredAt:  dutyRegistryBaseAt.Add(-time.Hour),
	}
}

func synVerificationRecord(t *testing.T, coverage domain.DutyCoverage, digest string) ports.DutyVerificationRecord {
	t.Helper()
	duty := viewValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	funds := viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01")
	scope := viewValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01")
	verification, err := domain.VerifyDutyPayment(duty, funds, scope,
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

// Covers: 资金事实引用往返——六件如实读回；同引用重登`已登记`不顶替；未登记 found=false。
func TestExternalFundsFactsRoundTripByReference(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	registration := synFundsFact(t, 12500)

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), registration)
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := store.LoadFundsFact(t.Context(), tenantA(t), registration.Fact)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	if loaded.Fact != registration.Fact || loaded.Source != "SYN-BANK-01" || loaded.Payer != "SYN-PAYER-01" ||
		loaded.Currency != "XTS" || loaded.AmountMinor != 12500 || !loaded.OccurredAt.Equal(registration.OccurredAt) {
		t.Fatalf("资金事实行走样：%+v", loaded)
	}

	replay, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenantA(t), synFundsFact(t, 99))
	})
	if err != nil || replay != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同引用重登该交回`已登记`：err=%v outcome=%v", err, replay)
	}
	if again, _, _ := store.LoadFundsFact(t.Context(), tenantA(t), registration.Fact); again.AmountMinor != 12500 {
		t.Fatalf("首版金额被顶替：%d", again.AmountMinor)
	}
	if _, found, err := store.LoadFundsFact(t.Context(), tenantA(t),
		viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-NOBODY")); err != nil || found {
		t.Fatalf("未登记该 found=false：err=%v found=%v", err, found)
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

	funds := `INSERT INTO customs_compliance.external_funds_fact
		(tenant_id, fact_ref, source_ref, payer_ref, currency, amount_minor, occurred_at, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	fixture.rejects(t, "负金额", funds,
		"tenant-a", "SYN-FUNDS-X", "SYN-BANK-01", "SYN-PAYER-01", "XTS", -1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "币种空白", funds,
		"tenant-a", "SYN-FUNDS-X", "SYN-BANK-01", "SYN-PAYER-01", " ", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)

	verification := `INSERT INTO customs_compliance.duty_payment_verification
		(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, coverage, delta, validity, basis, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	fixture.rejects(t, "没有资金事实的核对", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-NOBODY", "SYN-UNIT-X", "d", "COVERED", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
	fixture.seed(t, funds,
		"tenant-a", "SYN-FUNDS-X", "SYN-BANK-01", "SYN-PAYER-01", "XTS", 1, dutyRegistryBaseAt, dutyRegistryBaseAt)
	fixture.rejects(t, "无依据的核对", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "COVERED", "NO_DELTA", "VALID", "  ", dutyRegistryBaseAt)
	fixture.rejects(t, "覆盖轴集外", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "MAYBE", "NO_DELTA", "VALID", "basis", dutyRegistryBaseAt)
	fixture.rejects(t, "有效性轴集外", verification,
		"tenant-a", "SYN-DUTY-X", "SYN-FUNDS-X", "SYN-UNIT-X", "d", "COVERED", "NO_DELTA", "MAYBE", "basis", dutyRegistryBaseAt)
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
