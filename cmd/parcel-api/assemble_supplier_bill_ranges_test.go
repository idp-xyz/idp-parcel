package main

import (
	"net/http"
	"strings"
	"testing"
)

// 本文件对真实 PostgreSQL 证 ADR-0357 两处写口的生产装配：接收口的供应商账单号与审核口按范围给的三种结果。夹具同
// assemble_supplier_bill_decisions_test.go（一版 8500 CNY 的预期成本、审核授权、应付归集账户与 10 万上限）。证据只记 `S`。

func numberedSupplierBillBody(claim, number string) string {
	return strings.Replace(strings.Replace(supplierBillBody, `"SYN-BILL-PSB22-1"`, `"`+claim+`"`, 1),
		`"supplier":"SYN-PARTY-CARRIER-TRUNK-01",`, `"supplier":"SYN-PARTY-CARRIER-TRUNK-01","supplierBillNumber":"`+number+`",`, 1)
}

func rangedAuditBody(claim, decision, verdict, from, to, basis, resolves string) string {
	body := `{"claim":"` + claim + `","version":"v1","line":"L1","decision":"` + decision + `","verdict":"` + verdict +
		`","rangeFrom":"` + from + `","rangeTo":"` + to + `"`
	if basis != "" {
		body += `,"basis":"` + basis + `"`
	}
	if resolves != "" {
		body += `,"resolvesDispute":"` + resolves + `"`
	}
	return body + `}`
}

// Covers: AT-SA-233（生产装配半边）——接收口收供应商账单号、写进账单号册（分配方是该供应商）；同一供应商同一号换个主张标识再录答
// 200 账单号冲突、带出已登记的主张，不落第二份；不带号的旧载荷照收；新版本换了供应商答 200 主张供应商冲突、带出原先的供应商。
func TestTheReceptionFaceRegistersTheSupplierBillNumber(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)

	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, numberedSupplierBillBody("SYN-BILL-RANDOM-A", "INV-2610-001")); status != http.StatusCreated ||
		answer["outcome"] != "BILL_RECEIVED" {
		t.Fatalf("接收 = %d %v", status, answer)
	}
	if row := fixture.text(t, `SELECT supplier_ref || '|' || claim_id FROM settlement_accounting.supplier_bill_number
		 WHERE tenant_id = $1 AND bill_number = 'INV-2610-001'`, decisionsTenant); row != "SYN-PARTY-CARRIER-TRUNK-01|SYN-BILL-RANDOM-A" {
		t.Fatalf("账单号册 = %q", row)
	}
	status, answer := fixture.post(t, supplierBillReceptionFace, billSecondClerk, numberedSupplierBillBody("SYN-BILL-RANDOM-B", "INV-2610-001"))
	if status != http.StatusOK || answer["outcome"] != "SUPPLIER_BILL_NUMBER_CONFLICT" || answer["registeredClaim"] != "SYN-BILL-RANDOM-A" {
		t.Fatalf("同号另一份 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.supplier_bill_reception WHERE tenant_id = $1`, decisionsTenant); count != "1" {
		t.Fatalf("接收行 %s，同一张账单只该落一份", count)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("旧载荷 = %d %v", status, answer)
	}
	// 同一份主张的新版本换了供应商：答 200 主张供应商冲突、带出原先的供应商，不落库。
	switched := strings.Replace(strings.Replace(numberedSupplierBillBody("SYN-BILL-RANDOM-A", "INV-2610-001"), `"version":"v1"`, `"version":"v2"`, 1),
		`"supplier":"SYN-PARTY-CARRIER-TRUNK-01"`, `"supplier":"SYN-PARTY-CARRIER-TRUNK-02"`, 1)
	status, answer = fixture.post(t, supplierBillReceptionFace, billClerk, switched)
	if status != http.StatusOK || answer["outcome"] != "CLAIM_SUPPLIER_CONFLICT" || answer["registeredSupplier"] != "SYN-PARTY-CARRIER-TRUNK-01" ||
		answer["registeredSupplierBillNumber"] != nil {
		t.Fatalf("换供应商的新版本 = %d %v", status, answer)
	}
}

// Covers: AT-SA-234、AT-SA-235（生产装配半边）——8500 的一行拆成通过 5000、争议 3000、拒绝 500：只有通过的那一段答 201 带应付号与金额，
// 争议与拒绝答 201 不带号；重叠答 200 范围已裁决；争议的再审通过 2000 在同一行形成第二份应付、取下一个号，再审拒绝剩下的 1000；
// 重放争议答已有的决定；只点名应付标识的旧载荷撞上已审的段答范围已裁决。审核的人持审核应付与争议决定两种授予。
func TestTheAuditFaceGivesRangedVerdictsAndReaudits(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("先接收 = %d %v", status, answer)
	}
	const claim = "SYN-BILL-PSB22-1"

	status, answer := fixture.post(t, supplierBillAuditFace, billArbiter, rangedAuditBody(claim, "AUDIT-PASS", "PASSED", "0", "5000", "", ""))
	first, _ := answer["documentNumber"].(string)
	if status != http.StatusCreated || answer["outcome"] != "PAYABLE_FORMED" || answer["amount"] != "5000" || answer["payable"] != "AUDIT-PASS" ||
		answer["verdict"] != "PASSED" || answer["rangeFrom"] != "0" || answer["rangeTo"] != "5000" || !payableNumberPattern.MatchString(first) {
		t.Fatalf("通过 = %d %v", status, answer)
	}
	for _, step := range []struct {
		body    string
		status  int
		outcome string
	}{
		{rangedAuditBody(claim, "AUDIT-DISPUTE", "DISPUTED", "5000", "8000", "SYN-WEIGHT-PROOF", ""), http.StatusCreated, "DISPUTE_RECORDED"},
		{rangedAuditBody(claim, "AUDIT-REJECT", "REJECTED", "8000", "8500", "SYN-NOT-ORDERED", ""), http.StatusCreated, "REJECTION_RECORDED"},
		{rangedAuditBody(claim, "AUDIT-OVERLAP", "PASSED", "4000", "6000", "", ""), http.StatusOK, "RANGE_ALREADY_DECIDED"},
		{rangedAuditBody(claim, "AUDIT-DISPUTE", "DISPUTED", "5000", "8000", "SYN-WEIGHT-PROOF", ""), http.StatusOK, "EXISTING_DECISION"},
		{supplierBillAuditBody, http.StatusOK, "RANGE_ALREADY_DECIDED"},
	} {
		status, answer := fixture.post(t, supplierBillAuditFace, billArbiter, step.body)
		if status != step.status || answer["outcome"] != step.outcome || answer["documentNumber"] != nil {
			t.Fatalf("%s = %d %v，want %d %s", step.body, status, answer, step.status, step.outcome)
		}
	}

	status, answer = fixture.post(t, supplierBillAuditFace, billArbiter, rangedAuditBody(claim, "AUDIT-REPASS", "PASSED", "5000", "7000", "SYN-WEIGHT-CONFIRMED", "AUDIT-DISPUTE"))
	second, _ := answer["documentNumber"].(string)
	if status != http.StatusCreated || answer["outcome"] != "PAYABLE_FORMED" || answer["amount"] != "2000" ||
		answer["resolvesDispute"] != "AUDIT-DISPUTE" || !payableNumberPattern.MatchString(second) || second == first {
		t.Fatalf("再审通过 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billArbiter, rangedAuditBody(claim, "AUDIT-REREJECT", "REJECTED", "7000", "8000", "SYN-REFUSED", "AUDIT-DISPUTE")); status != http.StatusCreated ||
		answer["outcome"] != "REJECTION_RECORDED" {
		t.Fatalf("再审拒绝 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.audited_payable WHERE tenant_id = $1 AND claim_id = $2`, decisionsTenant, claim); count != "2" {
		t.Fatalf("同一行的应付 %s 份，want 2", count)
	}
	if evidence := fixture.text(t, `SELECT operation_evidence FROM settlement_accounting.supplier_bill_audit_decision
		 WHERE tenant_id = $1 AND decision_id = 'AUDIT-REJECT'`, decisionsTenant); evidence != operatorEvidenceOf(billArbiter) {
		t.Fatalf("拒绝决定的操作证据 = %q", evidence)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billClerk, rangedAuditBody(claim, "AUDIT-X", "REJECTED", "0", "1", "SYN-B", "")); status != http.StatusForbidden {
		t.Fatalf("持接收授予的人拒绝 = %d %v，want 403", status, answer)
	}
}

// Covers: AT-SA-234、AT-SA-235（生产装配半边，ADR-0357 决定二）——争议决定与审核应付分别授权：只持审核应付授予的人给争议、对争议再审
// 都答 403；只持争议决定授予的人能把一段挂成争议，通过、拒绝与再审答 403；两种都持的人再审通过形成应付。
func TestDisputeDecisionsAndAuditsAreGrantedSeparately(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("先接收 = %d %v", status, answer)
	}
	const claim = "SYN-BILL-PSB22-1"
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, rangedAuditBody(claim, "D-1", "DISPUTED", "0", "1000", "SYN-WEIGHT", "")); status != http.StatusForbidden {
		t.Fatalf("只持审核授予的人给争议 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billDisputer, rangedAuditBody(claim, "D-1", "DISPUTED", "0", "1000", "SYN-WEIGHT", "")); status != http.StatusCreated ||
		answer["outcome"] != "DISPUTE_RECORDED" {
		t.Fatalf("只持争议授予的人给争议 = %d %v", status, answer)
	}
	for name, body := range map[string]string{
		"pass":    rangedAuditBody(claim, "P-1", "PASSED", "1000", "2000", "", ""),
		"reject":  rangedAuditBody(claim, "R-1", "REJECTED", "1000", "2000", "SYN-NO", ""),
		"reaudit": rangedAuditBody(claim, "RE-1", "PASSED", "0", "1000", "SYN-OK", "D-1"),
		"legacy":  supplierBillAuditBody,
	} {
		if status, answer := fixture.post(t, supplierBillAuditFace, billDisputer, body); status != http.StatusForbidden {
			t.Fatalf("只持争议授予的人 %s = %d %v，want 403", name, status, answer)
		}
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, rangedAuditBody(claim, "RE-1", "PASSED", "0", "1000", "SYN-OK", "D-1")); status != http.StatusForbidden {
		t.Fatalf("只持审核授予的人再审 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billArbiter, rangedAuditBody(claim, "RE-1", "PASSED", "0", "1000", "SYN-OK", "D-1")); status != http.StatusCreated ||
		answer["outcome"] != "PAYABLE_FORMED" {
		t.Fatalf("两种都持的人再审通过 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, rangedAuditBody(claim, "P-2", "PASSED", "1000", "8500", "", "")); status != http.StatusCreated ||
		answer["outcome"] != "PAYABLE_FORMED" {
		t.Fatalf("只持审核授予的人通过 = %d %v", status, answer)
	}
}

// Covers: AT-SA-233（生产装配半边）——账单号首尾带空白答 400、不落接收行；大小写不同是另一个号，照常接收。
func TestTheReceptionFaceTakesTheBillNumberVerbatim(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, numberedSupplierBillBody("SYN-BILL-SPACE", "INV-9 ")); status != http.StatusBadRequest {
		t.Fatalf("尾带空格 = %d %v，want 400", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, numberedSupplierBillBody("SYN-BILL-UPPER", "INV-9")); status != http.StatusCreated {
		t.Fatalf("INV-9 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, numberedSupplierBillBody("SYN-BILL-LOWER", "inv-9")); status != http.StatusCreated {
		t.Fatalf("inv-9 = %d %v（大小写不折算，是另一个号）", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.supplier_bill_number WHERE tenant_id = $1`, decisionsTenant); count != "2" {
		t.Fatalf("账单号册 %s 行，want 2", count)
	}
}
