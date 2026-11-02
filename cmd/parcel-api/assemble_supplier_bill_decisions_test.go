package main

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证供应商账单接收口与审核口的生产装配（ADR-0340 决定一至三；票 product-strategy-boundary/22 第 3 笔）：两口按
// 特性登记处那一份 build 与 mount 装上，进的是 buildSettlementOrchestrations 装的那一只接收编排。一版供应商预期成本直写进册，审核授权、
// 应付归集账户与越权升级上限照受控登记口的载荷登。证据只记 `S`。

const (
	billClerk   = "SYN-BILL-CLERK-PSB22"
	billAuditor = "SYN-BILL-AUDITOR-PSB22"
	// billSecondClerk 另持一份账单接收授予，用来看换一个人重放留下的是谁。
	billSecondClerk = "SYN-BILL-CLERK-PSB22-B"

	supplierBillReceptionFace = "/settlement-supplier-bill-receptions"
	supplierBillAuditFace     = "/settlement-supplier-bill-audits"
)

func newSupplierBillFixture(t *testing.T) *settlementDecisionsFixture {
	t.Helper()
	fixture := newSettlementDecisionsFixture(t)
	operators, err := aipg.NewOperatorRegistry(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, billClerk, interval, accessidentity.DecisionSupplierBillReception)
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, billAuditor, interval, accessidentity.DecisionSupplierBillAudit)
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, billSecondClerk, interval, accessidentity.DecisionSupplierBillReception)
	for _, pattern := range []string{supplierBillReceptionFace, supplierBillAuditFace} {
		if fixture.endpoints[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}
	return fixture
}

// seedSupplierBillRegisters 登审核要的三本册与一版预期成本。auditor 为空时不登审核授权，看接收那一步怎么答。
func seedSupplierBillRegisters(t *testing.T, fixture *settlementDecisionsFixture, withAuthority bool) {
	t.Helper()
	catalogues, err := sapostgres.NewSettlementCatalogues(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	ceilings, err := sapostgres.NewAuditEscalationCeilings(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	costs, err := sapostgres.NewSupplierExpectedCosts(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	_, authority, err := registrationjson.SupplierAuditAuthorityFromJSON([]byte(
		`{"tenantId":"` + decisionsTenant + `","supplierId":"SYN-PARTY-CARRIER-TRUNK-01","legalEntityId":"SYN-LE-01","auditorId":"SYN-AUDITOR-ROLE-01"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, payableAccount, err := registrationjson.SupplierPayableAccountFromJSON([]byte(
		`{"tenantId":"` + decisionsTenant + `","supplierId":"SYN-PARTY-CARRIER-TRUNK-01","legalEntityId":"SYN-LE-01","currency":"CNY","accountId":"` + decisionsAccount + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, ceiling, err := registrationjson.AuditEscalationCeilingFromJSON([]byte(
		`{"tenantId":"` + decisionsTenant + `","supplierId":"SYN-PARTY-CARRIER-TRUNK-01","legalEntityId":"SYN-LE-01","currency":"CNY","ceilingMinor":100000}`))
	if err != nil {
		t.Fatal(err)
	}
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		mustSettlementValue(t, sadomain.NewChargeOccurrenceID, "SYN-TCO-PSB22-1"),
		mustSettlementValue(t, sadomain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		mustSettlementValue(t, sadomain.NewOccurrenceVersion, "V1"),
		time.Now().UTC().Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cny := mustSettlementValue(t, sadomain.NewCurrencyCode, "CNY")
	cost, err := sadomain.FormSupplierExpectedCost(sadomain.SupplierExpectedCostSpec{
		Version:            mustSettlementValue(t, sadomain.NewSupplierCostVersionID, "SYN-BUYSC-PSB22-1"),
		Occurrence:         occurrence,
		FeeItem:            mustSettlementValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-TRUNK-COST-01"),
		RuleVersion:        mustSettlementValue(t, sadomain.NewPurchaseRuleVersionReference, "SYN-PLAN-CN-SG-COST-01@v1"),
		Agreement:          mustSettlementValue(t, sadomain.NewSupplierAgreementReference, "SYN-SUPPLIER-TRUNK-CN-SG-01/v3"),
		Evaluation:         mustSettlementValue(t, sadomain.NewBuyEvaluationReference, "SYN-EVAL-BUY-PSB22-1"),
		OriginalCurrency:   cny,
		OriginalMinor:      8500,
		SettlementCurrency: cny,
		SettlementMinor:    8500,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if withAuthority {
			if _, err := catalogues.SaveSupplierAuditAuthority(ctx, fixture.tenant, authority, time.Now()); err != nil {
				return err
			}
		}
		if _, err := catalogues.SaveSupplierPayableAccount(ctx, fixture.tenant, payableAccount, time.Now()); err != nil {
			return err
		}
		if _, err := ceilings.SaveAuditEscalationCeiling(ctx, fixture.tenant, ceiling, time.Now()); err != nil {
			return err
		}
		_, err := costs.Save(ctx, fixture.tenant, cost, time.Now())
		return err
	}); err != nil {
		t.Fatalf("登夹具：%v", err)
	}
}

const supplierBillBody = `{"claim":"SYN-BILL-PSB22-1","version":"v1","supplier":"SYN-PARTY-CARRIER-TRUNK-01","legalEntity":"SYN-LE-01",` +
	`"period":"SYN-PERIOD-2026-10","currency":"CNY","lines":[{"line":"L1","feeItem":"SYN-FEE-ITEM-TRUNK-COST-01","claimed":"8500"}],` +
	`"receivedAt":"2026-10-28T02:00:00Z",` +
	`"directives":[{"line":"L1","classification":"MATCHED","expectedVersion":"SYN-BUYSC-PSB22-1"}]}`

const supplierBillAuditBody = `{"claim":"SYN-BILL-PSB22-1","version":"v1","line":"L1","payable":"SYN-PAYABLE-PSB22-1"}`

// Covers: AT-SA-225（ADR-0340 决定一、三）——持账单接收授予的人录一份账单并裁决匹配：答 201；另一位持同种授予的人重放答已存在，接收行
// 记下的仍是首次那一位；持审核授予的人打接收口答未授予。
func TestTheSupplierBillReceptionFaceReceivesAndRecordsTheClerk(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)

	if status, answer := fixture.post(t, supplierBillReceptionFace, billAuditor, supplierBillBody); status != http.StatusForbidden {
		t.Fatalf("持审核授予的人接收 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated ||
		answer["outcome"] != "BILL_RECEIVED" {
		t.Fatalf("接收 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billSecondClerk, supplierBillBody); status != http.StatusOK ||
		answer["outcome"] != "EXISTING_RESULT" {
		t.Fatalf("换一个人重放 = %d %v", status, answer)
	}
	if evidence := fixture.text(t, `SELECT operation_evidence FROM settlement_accounting.supplier_bill_reception
		 WHERE tenant_id = $1 AND claim_id = 'SYN-BILL-PSB22-1'`, decisionsTenant); evidence != operatorEvidenceOf(billClerk) {
		t.Fatalf("接收行的操作证据 = %q", evidence)
	}
}

// payableNumberPattern 是审核应付产品默认模板 AP-{yyyy}{mm}-{seq:5} 渲染出的形状（ADR-0355 决定五），比形状不比年月，理由同
// statementNumberPattern。
var payableNumberPattern = regexp.MustCompile(`^AP-\d{6}-\d{5}$`)

// Covers: AT-SA-226（ADR-0340 决定二、三）——持审核授予的人点一行：在上限内形成审核应付、答 201 带金额，重放答已存在；应付行记下点审核的
// 人，auditor_ref 仍是审核授权册上那一方；持接收授予的人打审核口答未授予。`AT-SA-231`（生产装配半边）：应付号由服务端在审核事务里
// 按编号规则取、随应付落库，重放答原号；载荷里的应付标识照旧是幂等键。
func TestTheSupplierBillAuditFaceFormsAPayableAndRecordsTheAuditor(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("先接收 = %d %v", status, answer)
	}

	if status, answer := fixture.post(t, supplierBillAuditFace, billClerk, supplierBillAuditBody); status != http.StatusForbidden {
		t.Fatalf("持接收授予的人审核 = %d %v，want 403", status, answer)
	}
	status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody)
	number, _ := answer["documentNumber"].(string)
	if status != http.StatusCreated || answer["outcome"] != "PAYABLE_FORMED" || answer["amount"] != "8500" || answer["currency"] != "CNY" ||
		answer["payable"] != "SYN-PAYABLE-PSB22-1" || !payableNumberPattern.MatchString(number) || !strings.HasSuffix(number, "-00001") {
		t.Fatalf("审核 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody); status != http.StatusOK ||
		answer["outcome"] != "EXISTING_PAYABLE" || answer["documentNumber"] != number {
		t.Fatalf("重放 = %d %v，要答原来那个应付号", status, answer)
	}
	if stored := fixture.text(t, `SELECT document_number FROM settlement_accounting.audited_payable
		 WHERE tenant_id = $1 AND payable_id = 'SYN-PAYABLE-PSB22-1'`, decisionsTenant); stored != number {
		t.Fatalf("库上应付号 = %q，want %s", stored, number)
	}
	if evidence := fixture.text(t, `SELECT operation_evidence FROM settlement_accounting.audited_payable
		 WHERE tenant_id = $1 AND payable_id = 'SYN-PAYABLE-PSB22-1'`, decisionsTenant); evidence != operatorEvidenceOf(billAuditor) {
		t.Fatalf("应付行的操作证据 = %q", evidence)
	}
	if auditor := fixture.text(t, `SELECT auditor_ref FROM settlement_accounting.audited_payable
		 WHERE tenant_id = $1 AND payable_id = 'SYN-PAYABLE-PSB22-1'`, decisionsTenant); auditor != "SYN-AUDITOR-ROLE-01" {
		t.Fatalf("auditor_ref = %q，want 审核授权册上那一方", auditor)
	}
}

// Covers: AT-SA-226（ADR-0340 决定二）——审核授权没登：接收照常成立并答复点明审核会停；审核答 200 带未配置原因，不形成应付。
func TestAnUnconfiguredAuditAuthorityIsAnAnswer(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, false)
	status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody)
	if status != http.StatusCreated || answer["auditAuthority"] != "UNCONFIGURED" {
		t.Fatalf("接收 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody); status != http.StatusOK ||
		answer["outcome"] != "AUDIT_UNDECIDED" || answer["undecidedReason"] != "AUDIT_AUTHORITY_UNCONFIGURED" {
		t.Fatalf("审核 = %d %v", status, answer)
	}
}

// Covers: `AT-SA-231`（生产装配半边）——租户换了一份编号格式，新格式下跳号上限（docnumber.MaxSkippedNumbers）以内的号都已被换格式前的应付占用：审核答「编号规则撞号」、
// 带出该改哪一类的编号规则，不形成应付；生产事务壳整笔回滚、取过的流水随之释放——腾出第一个号之后再审，拿到的正是它。
func TestAPayableNumberCollisionFormsNothingAndReleasesTheSequence(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("先接收 = %d %v", status, answer)
	}
	if _, err := fixture.numbers.Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: decisionsTenant, Type: sadomain.AuditedPayableDocumentNumberType.Code, Template: "OLDAP{seq:2}",
		EffectiveFrom: time.Now().Add(-time.Hour), RegisteredBy: "SYN-TEST",
	}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := fixture.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id, currency, amount_minor,
			 auditor_ref, audited_at, content_digest, recorded_at, document_number)
			SELECT $1, 'SYN-PAYABLE-OLD-' || n, 'SYN-BILL-OLD', 'L' || n, 'SYN-COST-OLD', 'SYN-LE-01', $2, 'CNY', 100,
			       'SYN-AUDITOR-ROLE-01', now(), 'digest-old-' || n, now(), 'OLDAP' || lpad(n::text, 2, '0')
			  FROM generate_series(1, $3::int) AS n`, decisionsTenant, decisionsAccount, docnumber.MaxSkippedNumbers)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody)
	if status != http.StatusOK || answer["outcome"] != "NUMBERING_RULE_COLLISION" || answer["documentType"] != "AUDITED_PAYABLE" ||
		answer["payable"] != nil {
		t.Fatalf("撞号 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.audited_payable
		 WHERE tenant_id = $1 AND payable_id = 'SYN-PAYABLE-PSB22-1'`, decisionsTenant); count != "0" {
		t.Fatalf("撞号还形成了应付：%s", count)
	}

	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := fixture.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `DELETE FROM settlement_accounting.audited_payable WHERE tenant_id = $1 AND document_number = 'OLDAP01'`, decisionsTenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody); status != http.StatusCreated ||
		answer["documentNumber"] != "OLDAP01" {
		t.Fatalf("腾出第一个号之后 = %d %v，撞号那一次取过的流水没随回滚释放", status, answer)
	}
}
