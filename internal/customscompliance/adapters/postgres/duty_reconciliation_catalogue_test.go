package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newDutyReconciliationCatalogue(t *testing.T) (*adapter.DutyReconciliationCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewDutyReconciliationCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造税费付款两册读口：%v", err)
	}
	return catalogue, fixture
}

func listCollaborations(
	t *testing.T,
	catalogue *adapter.DutyReconciliationCatalogue,
	tenant string,
	limit int,
) []domain.DutyPaymentCollaboration {
	t.Helper()
	entries, err := catalogue.ListDutyCollaborations(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列协作事项册：%v", err)
	}
	return entries
}

func listVerifications(
	t *testing.T,
	catalogue *adapter.DutyReconciliationCatalogue,
	tenant string,
	limit int,
) []ports.DutyVerificationRecord {
	t.Helper()
	records, err := catalogue.ListDutyVerifications(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列付款核对册：%v", err)
	}
	return records
}

// seedFundsFact 播一条资金事实引用：核对表外键钉「没有接收的资金事实就没有核对」，核对
// 行播种前它必须在场（0016 自注）。
func seedFundsFact(t *testing.T, fixture *viewFixture, tenant, fact string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.external_funds_fact
			(tenant_id, fact_ref, source_ref, payer_ref, currency, amount_minor, occurred_at, received_at)
		 VALUES ($1, $2, 'SYN-BANK-01', 'SYN-PAYER-01', 'XTS', 12500, $3, $3)`,
		tenant, fact, viewBaseAt)
}

// Covers: 票 sa-cc/10 重点 ⑤ — 必填口构造期拒 nil。
func TestDutyReconciliationCatalogueRefusesANilDB(t *testing.T) {
	if _, err := adapter.NewDutyReconciliationCatalogue(nil); err == nil {
		t.Fatal("nil db 被接受了")
	}
}

// Covers: ADR-0077 Decision 四 — 两册空时各自如实答空列表，不是错误也不折成未配置。
func TestEmptyDutyRegistersAnswerEmptyLists(t *testing.T) {
	catalogue, _ := newDutyReconciliationCatalogue(t)
	if entries := listCollaborations(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 份协作事项", len(entries))
	}
	if records := listVerifications(t, catalogue, "tenant-a", 10); len(records) != 0 {
		t.Fatalf("空库上列出 %d 版核对", len(records))
	}
}

// Covers: 0016 自注 / CONTEXT「税费付款协作事项」— 义务依据两格各占一行、逐格如实读回：
// 核定税费格带税费引用不带无需付款依据，明确无需付款格反之；跨租户不可见；按（范围，
// 税费引用）键序稳定。
func TestDutyCollaborationCatalogueListsBothKindsVerbatim(t *testing.T) {
	catalogue, fixture := newDutyReconciliationCatalogue(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.duty_payment_collaboration
			(tenant_id, scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at)
		 VALUES
			('tenant-a', 'SYN-UNIT-02', 'SYN-DUTY-02/v1', 'ASSESSED_DUTY', '', 'SYN-OBLIGOR-02', 'SYN-ASSESSMENT-02', 'SYN-DUTY-DESK', $1),
			('tenant-a', 'SYN-UNIT-01', '', 'EXPLICITLY_NOT_REQUIRED', 'SYN-PROGRAM-01: no duty on this scope', 'SYN-OBLIGOR-01', 'SYN-PROGRAM-01', 'SYN-DUTY-DESK', $1),
			('tenant-b', 'SYN-UNIT-09', 'SYN-DUTY-09/v1', 'ASSESSED_DUTY', '', 'SYN-OBLIGOR-09', 'SYN-ASSESSMENT-09', 'SYN-OTHER-DESK', $1)`,
		viewBaseAt)

	entries := listCollaborations(t, catalogue, "tenant-a", 10)
	if len(entries) != 2 {
		t.Fatalf("上列了 %d 份协作事项，要 2 份：%+v", len(entries), entries)
	}
	notRequired := entries[0]
	basis, hasBasis := notRequired.NoPayBasis()
	if _, hasDuty := notRequired.Duty(); notRequired.Kind() != domain.ObligationExplicitlyNotRequired ||
		hasDuty || !hasBasis || basis != "SYN-PROGRAM-01: no duty on this scope" ||
		notRequired.Scope().String() != "SYN-UNIT-01" {
		t.Fatalf("无需付款格走样：%+v", notRequired)
	}
	assessed := entries[1]
	duty, hasDuty := assessed.Duty()
	if _, hasBasis := assessed.NoPayBasis(); assessed.Kind() != domain.ObligationFromAssessedDuty ||
		!hasDuty || hasBasis || duty.String() != "SYN-DUTY-02/v1" ||
		assessed.Scope().String() != "SYN-UNIT-02" || assessed.Obligor().String() != "SYN-OBLIGOR-02" ||
		assessed.Requirement().String() != "SYN-ASSESSMENT-02" || assessed.Target().String() != "SYN-DUTY-DESK" ||
		!assessed.FormedAt().Equal(viewBaseAt) {
		t.Fatalf("核定税费格走样：%+v", assessed)
	}
	for _, entry := range entries {
		if entry.Scope().String() == "SYN-UNIT-09" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0137 决定三 / CONTEXT「不能实现为一组互斥总状态」 — 三轴逐格原值读回，不折总状态；同三维键的
// 多版本连指纹与依据一起全部上列（迟到事实按新版本追加，不按到达顺序覆盖）；跨租户不可见；
// 版本按核对时间升序。
func TestDutyVerificationCatalogueListsEveryVersionWithItsThreeAxes(t *testing.T) {
	catalogue, fixture := newDutyReconciliationCatalogue(t)
	seedFundsFact(t, fixture, "tenant-a", "SYN-FUNDS-01")
	seedFundsFact(t, fixture, "tenant-b", "SYN-FUNDS-09")
	later := viewBaseAt.Add(2 * time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.duty_payment_verification
			(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, coverage, delta, validity, basis, verified_at)
		 VALUES
			('tenant-a', 'SYN-DUTY-01/v1', 'SYN-FUNDS-01', 'SYN-UNIT-01', 'digest-v2', 'COVERED', 'NO_DELTA', 'VALID',   'SYN-RULE-01: remittance quotes assessment', $2),
			('tenant-a', 'SYN-DUTY-01/v1', 'SYN-FUNDS-01', 'SYN-UNIT-01', 'digest-v1', 'PARTIAL', 'SHORT',    'PENDING', 'SYN-RULE-01: remittance quotes assessment', $1),
			('tenant-b', 'SYN-DUTY-09/v1', 'SYN-FUNDS-09', 'SYN-UNIT-09', 'digest-x',  'NONE',    'PENDING',  'CONFLICTING', 'SYN-RULE-09', $1)`,
		viewBaseAt, later)

	records := listVerifications(t, catalogue, "tenant-a", 10)
	if len(records) != 2 {
		t.Fatalf("上列了 %d 版核对，要 2 版：%+v", len(records), records)
	}
	first, second := records[0], records[1]
	if first.Key.Digest != "digest-v1" || second.Key.Digest != "digest-v2" {
		t.Fatalf("同键版本没按核对时间升序：%q, %q", first.Key.Digest, second.Key.Digest)
	}
	if first.Key.TenantID.String() != "tenant-a" || first.Key.Duty.String() != "SYN-DUTY-01/v1" ||
		first.Key.Funds.String() != "SYN-FUNDS-01" || first.Key.Scope.String() != "SYN-UNIT-01" {
		t.Fatalf("幂等键三维走样：%+v", first.Key)
	}
	if first.Verification.Coverage() != domain.CoveragePartial || first.Verification.Delta() != domain.DeltaShort ||
		first.Verification.Validity() != domain.FundsFactPending || !first.Verification.VerifiedAt().Equal(viewBaseAt) {
		t.Fatalf("首版三轴走样（要 PARTIAL / SHORT / PENDING 逐格原值）：%+v", first.Verification)
	}
	if second.Verification.Coverage() != domain.CoverageFull || second.Verification.Delta() != domain.DeltaNone ||
		second.Verification.Validity() != domain.FundsFactValid || !second.Verification.VerifiedAt().Equal(later) {
		t.Fatalf("次版三轴走样（要 COVERED / NO_DELTA / VALID 逐格原值）：%+v", second.Verification)
	}
	if first.Basis != "SYN-RULE-01: remittance quotes assessment" {
		t.Fatalf("关联依据走样：%q", first.Basis)
	}
	for _, record := range records {
		if record.Key.Scope.String() == "SYN-UNIT-09" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0077 Decision 五 — 两口 limit 非正各自拒；limit 截断行数而不是静默全量。
func TestDutyRegisterCataloguesGuardTheirLimits(t *testing.T) {
	catalogue, fixture := newDutyReconciliationCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	for _, limit := range []int{0, -1} {
		if _, err := catalogue.ListDutyCollaborations(t.Context(), tenant, limit); err == nil {
			t.Fatalf("limit=%d 的协作事项上列被接受了", limit)
		}
		if _, err := catalogue.ListDutyVerifications(t.Context(), tenant, limit); err == nil {
			t.Fatalf("limit=%d 的付款核对上列被接受了", limit)
		}
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.duty_payment_collaboration
			(tenant_id, scope_ref, duty_ref, kind, no_pay_basis, obligor_ref, requirement_ref, target_ref, formed_at)
		 VALUES
			('tenant-a', 'SYN-UNIT-01', 'SYN-DUTY-01/v1', 'ASSESSED_DUTY', '', 'SYN-OBLIGOR-01', 'SYN-ASSESSMENT-01', 'SYN-DUTY-DESK', $1),
			('tenant-a', 'SYN-UNIT-02', 'SYN-DUTY-02/v1', 'ASSESSED_DUTY', '', 'SYN-OBLIGOR-01', 'SYN-ASSESSMENT-02', 'SYN-DUTY-DESK', $1),
			('tenant-a', 'SYN-UNIT-03', 'SYN-DUTY-03/v1', 'ASSESSED_DUTY', '', 'SYN-OBLIGOR-01', 'SYN-ASSESSMENT-03', 'SYN-DUTY-DESK', $1)`,
		viewBaseAt)
	if entries := listCollaborations(t, catalogue, "tenant-a", 2); len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 份协作事项", len(entries))
	}

	seedFundsFact(t, fixture, "tenant-a", "SYN-FUNDS-01")
	fixture.seed(t,
		`INSERT INTO customs_compliance.duty_payment_verification
			(tenant_id, duty_ref, funds_ref, scope_ref, version_digest, coverage, delta, validity, basis, verified_at)
		 VALUES
			('tenant-a', 'SYN-DUTY-01/v1', 'SYN-FUNDS-01', 'SYN-UNIT-01', 'digest-1', 'NONE', 'PENDING', 'PENDING', 'SYN-RULE-01', $1),
			('tenant-a', 'SYN-DUTY-01/v1', 'SYN-FUNDS-01', 'SYN-UNIT-01', 'digest-2', 'NONE', 'PENDING', 'PENDING', 'SYN-RULE-01', $1),
			('tenant-a', 'SYN-DUTY-01/v1', 'SYN-FUNDS-01', 'SYN-UNIT-01', 'digest-3', 'NONE', 'PENDING', 'PENDING', 'SYN-RULE-01', $1)`,
		viewBaseAt)
	if records := listVerifications(t, catalogue, "tenant-a", 2); len(records) != 2 {
		t.Fatalf("limit=2 却上列了 %d 版核对", len(records))
	}
}
