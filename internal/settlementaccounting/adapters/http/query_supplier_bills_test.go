package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 供应商账单与应付页两个读口的转写用例（票 document-numbering/11）。

type stubSupplierBillLines struct {
	found ports.SupplierBillLines
	has   bool
	err   error

	gotTenant, gotClaim, gotVersion string
}

func (stub *stubSupplierBillLines) FindSupplierBillLines(
	_ context.Context, tenant domain.TenantID, claim, version string,
) (ports.SupplierBillLines, bool, error) {
	stub.gotTenant, stub.gotClaim, stub.gotVersion = tenant.String(), claim, version
	return stub.found, stub.has, stub.err
}

type stubAuditedPayables struct {
	rows []ports.AuditedPayableCatalogueRow
	err  error

	gotTenant string
	gotFilter ports.AuditedPayableFilter
	gotLimit  int
}

// ListAuditedPayables 像真读口一样截在 limit，理由同 stubStatementCatalogue。
func (stub *stubAuditedPayables) ListAuditedPayables(
	_ context.Context, tenant domain.TenantID, filter ports.AuditedPayableFilter, limit int,
) ([]ports.AuditedPayableCatalogueRow, error) {
	stub.gotTenant, stub.gotFilter, stub.gotLimit = tenant.String(), filter, limit
	if len(stub.rows) > limit {
		return stub.rows[:limit], stub.err
	}
	return stub.rows, stub.err
}

// Covers: AT-SA-232、AT-SA-234 — 一份账单的各行逐格透出：匹配分类原名、所引预期成本与差额、依据、已匹配金额与四个和、这一行上
// 的各份审核决定（结果、范围与金额、所解争议、审核授权方与点审核的人分开；通过的带应付，应付单号与无号原因恰出其一）；没有对应
// 发生项的行不出预期与差额，没有决定的行 decisions 是空数组；供应商账单号照实透出；租户只取自接入面。
func TestSupplierBillLinesQueryTranscribesMatchAndPayable(t *testing.T) {
	matchedAt := catalogueBaseAt.Add(time.Minute)
	stub := &stubSupplierBillLines{has: true, found: ports.SupplierBillLines{
		Claim: "SYN-SUP-01/INV-1", ClaimVersion: "1", Supplier: "SYN-SUP-01", SupplierBillNumber: "INV-1", LegalEntity: "SYN-LE-01", Period: "2026-08",
		Currency: "CNY", ReceivedAt: catalogueBaseAt, AuditAuthorityConfigured: false, RecordedAt: catalogueBaseAt,
		Lines: []ports.SupplierBillLineEntry{
			{
				Line: "1", FeeItem: "SYN-FEE-01", ClaimedMinor: 1000, Classification: "MATCHED", ExpectedVersion: "SYN-EXP-1",
				ExpectedMinor: 1000, MatchedAt: &matchedAt, MatchedMinor: 1000, PassedMinor: 600, OpenDisputeMinor: 400,
				Decisions: []ports.SupplierBillLineDecision{
					{
						Decision: "PAYABLE-1", Verdict: "PASSED", RangeFromMinor: 0, RangeToMinor: 600, ClaimVersion: "1",
						AuditorRef: "SYN-AUDITOR-ROLE", OperationEvidence: "OPERATOR/syn#auditor", AuditedAt: matchedAt,
						Payable: &ports.SupplierBillLinePayable{Payable: "PAYABLE-1", DocumentNumber: "AP-202608-00001", AmountMinor: 600, Currency: "CNY"},
					},
					{
						Decision: "AUDIT-2", Verdict: "DISPUTED", RangeFromMinor: 600, RangeToMinor: 1000, Basis: "重量待核",
						AuditorRef: "SYN-AUDITOR-ROLE", AuditedAt: matchedAt,
					},
				},
			},
			{Line: "2", FeeItem: "SYN-FEE-02", ClaimedMinor: 2000, Classification: "PRICE_VARIANCE", ExpectedVersion: "SYN-EXP-2",
				ExpectedMinor: 1800, Basis: "合同价 18 元", MatchedAt: &matchedAt},
			{Line: "3", FeeItem: "SYN-FEE-03", ClaimedMinor: 300, Classification: "NO_MATCHING_OCCURRENCE", Basis: "查过 8 月干线",
				MatchedAt: &matchedAt},
		},
	}}
	response := serveGet(settlementhttp.NewQuerySupplierBillLinesEndpoint(grantedIntake(), stub),
		"/settlement-supplier-bill-lines?claim=SYN-SUP-01%2FINV-1&version=1&tenant=TENANT-9")
	if response.Code != http.StatusOK || stub.gotTenant != "TENANT-1" || stub.gotClaim != "SYN-SUP-01/INV-1" || stub.gotVersion != "1" {
		t.Fatalf("status = %d tenant = %q claim = %q version = %q", response.Code, stub.gotTenant, stub.gotClaim, stub.gotVersion)
	}
	var body struct {
		Outcome                  string `json:"outcome"`
		SupplierBillNumber       string `json:"supplierBillNumber"`
		AuditAuthorityConfigured *bool  `json:"auditAuthorityConfigured"`
		Lines                    []map[string]json.RawMessage
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "SUPPLIER_BILL_LINES_LISTED" || body.SupplierBillNumber != "INV-1" || body.AuditAuthorityConfigured == nil ||
		*body.AuditAuthorityConfigured || len(body.Lines) != 3 {
		t.Fatalf("账单面走样（审核授权没配也要说出 false）：%s", response.Body.String())
	}
	text := func(line map[string]json.RawMessage, key string) string {
		var value string
		if raw, present := line[key]; present {
			_ = json.Unmarshal(raw, &value)
		}
		return value
	}
	matched, variance, missing := body.Lines[0], body.Lines[1], body.Lines[2]
	if text(matched, "classification") != "MATCHED" || text(matched, "expectedAmount") != "1000" || text(matched, "varianceAmount") != "0" {
		t.Fatalf("已匹配行走样：%s", response.Body.String())
	}
	if text(matched, "matchedAmount") != "1000" || text(matched, "passedAmount") != "600" || text(matched, "openDisputeAmount") != "400" ||
		text(matched, "undecidedAmount") != "0" {
		t.Fatalf("已匹配金额或四个和走样：%s", response.Body.String())
	}
	var decisions []struct {
		Decision          string            `json:"decision"`
		Verdict           string            `json:"verdict"`
		RangeFrom         string            `json:"rangeFrom"`
		RangeTo           string            `json:"rangeTo"`
		Amount            string            `json:"amount"`
		Basis             string            `json:"basis"`
		Auditor           string            `json:"auditor"`
		OperationEvidence string            `json:"operationEvidence"`
		Payable           map[string]string `json:"payable"`
	}
	if err := json.Unmarshal(matched["decisions"], &decisions); err != nil || len(decisions) != 2 {
		t.Fatalf("已匹配行要带两份决定：%s", response.Body.String())
	}
	passed, disputed := decisions[0], decisions[1]
	if passed.Verdict != "PASSED" || passed.RangeFrom != "0" || passed.RangeTo != "600" || passed.Amount != "600" ||
		passed.Auditor != "SYN-AUDITOR-ROLE" || passed.OperationEvidence != "OPERATOR/syn#auditor" ||
		passed.Payable["documentNumber"] != "AP-202608-00001" || passed.Payable["payable"] != "PAYABLE-1" || passed.Payable["amount"] != "600" {
		t.Fatalf("通过那一份走样：%+v", passed)
	}
	if _, present := passed.Payable["documentNumberAbsence"]; present {
		t.Fatalf("有号就不出无号原因：%v", passed.Payable)
	}
	if disputed.Verdict != "DISPUTED" || disputed.Basis != "重量待核" || disputed.Payable != nil || disputed.OperationEvidence != "" {
		t.Fatalf("争议那一份走样：%+v", disputed)
	}
	if text(variance, "varianceAmount") != "200" || text(variance, "basis") != "合同价 18 元" {
		t.Fatalf("价差行的差额要是主张减预期：%s", response.Body.String())
	}
	if string(variance["decisions"]) != "[]" {
		t.Fatalf("没有决定的行 decisions 要是空数组：%s", response.Body.String())
	}
	for _, absent := range []string{"expectedVersion", "expectedAmount", "varianceAmount"} {
		if _, present := missing[absent]; present {
			t.Fatalf("没有对应发生项的行不该带 %q：%s", absent, response.Body.String())
		}
	}
}

// Covers: AT-SA-232 — 主张或版本缺即坏请求、读口不被触到；不在本租户册上答 SUPPLIER_BILL_NOT_FOUND（200）；读不回答
// NO_ANSWER_FORMED，不答「没有这份账单」。
func TestSupplierBillLinesQuerySeparatesMissingFromUnanswered(t *testing.T) {
	for _, target := range []string{"/settlement-supplier-bill-lines?claim=SYN-SUP-01%2FINV-1", "/settlement-supplier-bill-lines?version=1"} {
		response := serveGet(settlementhttp.NewQuerySupplierBillLinesEndpoint(grantedIntake(), unreachableCatalogues{t: t}), target)
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：status = %d body = %s", target, response.Code, response.Body.String())
		}
	}
	target := "/settlement-supplier-bill-lines?claim=SYN-SUP-01%2FINV-1&version=1"
	missing := serveGet(settlementhttp.NewQuerySupplierBillLinesEndpoint(grantedIntake(), &stubSupplierBillLines{}), target)
	if missing.Code != http.StatusOK || !strings.Contains(missing.Body.String(), `"outcome":"SUPPLIER_BILL_NOT_FOUND"`) ||
		strings.Contains(missing.Body.String(), "lines") {
		t.Fatalf("不在册要答 SUPPLIER_BILL_NOT_FOUND 且不带行：%d %s", missing.Code, missing.Body.String())
	}
	failed := serveGet(settlementhttp.NewQuerySupplierBillLinesEndpoint(grantedIntake(), &stubSupplierBillLines{err: errors.New("db down")}), target)
	if failed.Code != http.StatusInternalServerError || problemCode(t, failed) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回要答 NO_ANSWER_FORMED：%d %s", failed.Code, failed.Body.String())
	}
}

// Covers: AT-SA-232、AT-SA-231 — 审核应付册逐笔透出应付单号或无号原因（恰出其一）、所属主张与行、供应商与账期、审核授权方与
// 点审核的人；检索维原样交给读口，不认的参数不采信；多于一页答 truncated、只列一页；空册答空数组；读不回答 NO_ANSWER_FORMED。
func TestAuditedPayablesQueryTranscribesNumbersAndPassesFilters(t *testing.T) {
	stub := &stubAuditedPayables{rows: []ports.AuditedPayableCatalogueRow{
		{Payable: "PAYABLE-1", DocumentNumber: "AP-202608-00001", Claim: "SYN-SUP-01/INV-1", Line: "1", ExpectedVersion: "SYN-EXP-1",
			Supplier: "SYN-SUP-01", Period: "2026-08", LegalEntity: "SYN-LE-01", Account: "SYN-ACC-PAYABLE", Currency: "CNY",
			AmountMinor: 1000, AuditorRef: "SYN-AUDITOR-ROLE", OperationEvidence: "OPERATOR/syn#auditor",
			AuditedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt},
		{Payable: "PAYABLE-2", DocumentNumberAbsence: "ISSUED_BEFORE_NUMBERING", Claim: "SYN-SUP-01/INV-0", Line: "1",
			ExpectedVersion: "SYN-EXP-0", Supplier: "SYN-SUP-01", Period: "2026-07", LegalEntity: "SYN-LE-01", Account: "SYN-ACC-PAYABLE",
			Currency: "CNY", AmountMinor: 500, AuditorRef: "SYN-AUDITOR-ROLE", AuditedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt},
	}}
	response := serveGet(settlementhttp.NewQueryAuditedPayablesEndpoint(grantedIntake(), stub),
		"/settlement-audited-payables?documentNumber=AP-202608-00001&claim=SYN-SUP-01%2FINV-1&supplier=SYN-SUP-01&tenant=TENANT-9")
	// 向读口多要一行，只为判页外还有没有。
	if response.Code != http.StatusOK || stub.gotTenant != "TENANT-1" || stub.gotLimit != 26 {
		t.Fatalf("status = %d tenant = %q limit = %d", response.Code, stub.gotTenant, stub.gotLimit)
	}
	if stub.gotFilter != (ports.AuditedPayableFilter{DocumentNumber: "AP-202608-00001", Claim: "SYN-SUP-01/INV-1", Supplier: "SYN-SUP-01"}) {
		t.Fatalf("检索维没原样交给读口：%+v", stub.gotFilter)
	}
	var body struct {
		Outcome   string              `json:"outcome"`
		Truncated *bool               `json:"truncated"`
		Payables  []map[string]string `json:"payables"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "AUDITED_PAYABLES_LISTED" || len(body.Payables) != 2 || body.Truncated == nil || *body.Truncated {
		t.Fatalf("应付册走样：%s", response.Body.String())
	}
	numbered, before := body.Payables[0], body.Payables[1]
	if numbered["documentNumber"] != "AP-202608-00001" || numbered["supplier"] != "SYN-SUP-01" || numbered["amount"] != "1000" ||
		numbered["auditor"] != "SYN-AUDITOR-ROLE" || numbered["operationEvidence"] != "OPERATOR/syn#auditor" || numbered["claim"] != "SYN-SUP-01/INV-1" {
		t.Fatalf("有号那一笔走样：%v", numbered)
	}
	if _, present := numbered["documentNumberAbsence"]; present {
		t.Fatalf("有号就不出无号原因：%v", numbered)
	}
	if before["documentNumberAbsence"] != "ISSUED_BEFORE_NUMBERING" {
		t.Fatalf("没号要说原因：%v", before)
	}
	for _, absent := range []string{"documentNumber", "operationEvidence"} {
		if _, present := before[absent]; present {
			t.Fatalf("没号、不记提交方的那一笔不该带 %q：%v", absent, before)
		}
	}

	many := make([]ports.AuditedPayableCatalogueRow, 30)
	for index := range many {
		many[index] = ports.AuditedPayableCatalogueRow{Payable: fmt.Sprintf("PAYABLE-%03d", index), AuditedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt}
	}
	cut := serveGet(settlementhttp.NewQueryAuditedPayablesEndpoint(grantedIntake(), &stubAuditedPayables{rows: many}), "/settlement-audited-payables")
	var cutBody struct {
		Truncated bool              `json:"truncated"`
		Payables  []json.RawMessage `json:"payables"`
	}
	if err := json.Unmarshal(cut.Body.Bytes(), &cutBody); err != nil || !cutBody.Truncated || len(cutBody.Payables) != 25 {
		t.Fatalf("多于一页要答 truncated、只列一页：%s", cut.Body.String())
	}

	empty := serveGet(settlementhttp.NewQueryAuditedPayablesEndpoint(grantedIntake(), &stubAuditedPayables{}), "/settlement-audited-payables")
	if empty.Code != http.StatusOK || arrayAt(t, empty, "payables") != "[]" {
		t.Fatalf("空册要答空数组：%d %s", empty.Code, empty.Body.String())
	}
	failed := serveGet(settlementhttp.NewQueryAuditedPayablesEndpoint(grantedIntake(), &stubAuditedPayables{err: errors.New("db down")}),
		"/settlement-audited-payables")
	if failed.Code != http.StatusInternalServerError || problemCode(t, failed) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回要答 NO_ANSWER_FORMED，不答空册：%d %s", failed.Code, failed.Body.String())
	}
}
