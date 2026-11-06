package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// stubStatementCatalogue 是对账页两本册子的读口替身。
type stubStatementCatalogue struct {
	statements []ports.CustomerStatementCatalogueRow
	receptions []ports.SupplierBillReceptionCatalogueRow
	err        error

	gotTenant          string
	gotLimit           int
	gotReceptionFilter ports.SupplierBillReceptionFilter
}

func (stub *stubStatementCatalogue) ListCustomerStatements(
	_ context.Context, tenant domain.TenantID, limit int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return cataloguepage.Result[ports.CustomerStatementCatalogueRow]{Rows: stub.statements, Total: int64(len(stub.statements))}, stub.err
}

// ListSupplierBillReceptions 像真读口一样截在 limit、页外还有时给出游标：端点的 truncated 只看游标在不在，替身不截就证不了那一判。
func (stub *stubStatementCatalogue) ListSupplierBillReceptions(
	_ context.Context, tenant domain.TenantID, filter ports.SupplierBillReceptionFilter, limit int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
	stub.gotTenant, stub.gotLimit, stub.gotReceptionFilter = tenant.String(), limit, filter
	page := cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow]{Rows: stub.receptions, Total: int64(len(stub.receptions))}
	if len(stub.receptions) > limit {
		page.Rows, page.Next = stub.receptions[:limit], "SYN-CURSOR-NEXT"
	}
	return page, stub.err
}

func statementsEndpoint(register *stubStatementCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), register)
}

// Covers: 逐格转写 — 单号、账户、周期、总额与两个计数原样透出；异议与后续纳入以各自
// 数组挂在单上；未裁定的异议不带裁定三键（不代填「待处理」）；纳入行没有金额键。`AT-SA-230`（读口半边）：
// 每项争议带争议号，或带它为什么没有。查询串里自报的租户是集外键，答 400 不带任何行（ADR-0144 决定四）。
func TestCustomerStatementQueryAttachesDisputesAndInclusions(t *testing.T) {
	resolvedAt := catalogueBaseAt.Add(72 * time.Hour)
	register := &stubStatementCatalogue{
		statements: []ports.CustomerStatementCatalogueRow{
			{
				StatementNumber: "SYN-STMT-01",
				Account:         "SYN-ACC-01",
				Period:          "2026-07",
				Currency:        "SYN-CUR-01",
				TotalMinor:      250000,
				LineCount:       4,
				AdjustmentCount: 1,
				PublishedAt:     catalogueBaseAt,
				Disputes: []ports.StatementDisputeEntry{
					{
						Dispute:        "SYN-DISP-RESOLVED",
						DocumentNumber: "DSP-202607-00003",
						Charge:         "SYN-CHG-01",
						DisputedMinor:  12000,
						Reason:         "WEIGHT_MISMATCH",
						OpenedAt:       catalogueBaseAt.Add(24 * time.Hour),
						Resolution:     "UPHELD",
						ResolutionRef:  "SYN-RES-01",
						ResolvedAt:     &resolvedAt,
					},
					{
						Dispute:               "SYN-DISP-OPEN",
						DocumentNumberAbsence: "ISSUED_BEFORE_NUMBERING",
						Charge:                "SYN-CHG-02",
						DisputedMinor:         8000,
						Reason:                "RATE_DISPUTE",
						OpenedAt:              catalogueBaseAt.Add(24 * time.Hour),
					},
				},
				SubsequentInclusions: []ports.SubsequentInclusionEntry{
					{
						Inclusion:        "SYN-INC-ADJ",
						Kind:             "ADJUSTMENT",
						OriginalPeriod:   "2026-06",
						SubsequentPeriod: "2026-07",
						Charge:           "SYN-CHG-03",
						Adjustment:       "SYN-ADJ-01",
						IncludedAt:       catalogueBaseAt,
					},
					{
						Inclusion:        "SYN-INC-LATE",
						Kind:             "LATE_CHARGE",
						OriginalPeriod:   "2026-06",
						SubsequentPeriod: "2026-07",
						Charge:           "SYN-CHG-04",
						IncludedAt:       catalogueBaseAt,
					},
				},
			},
		},
	}
	selfReported := serveGet(statementsEndpoint(register), "/settlement-statements?registry=customer-statement&tenant=TENANT-9")
	assertPageQueryRefused(t, selfReported)
	assertNoOutcome(t, selfReported)
	if register.gotTenant != "" {
		t.Fatalf("自报租户的请求触到了读口：tenant=%q", register.gotTenant)
	}

	response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=customer-statement")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome    string `json:"outcome"`
		Statements []struct {
			StatementNumber      string                       `json:"statementNumber"`
			Account              string                       `json:"account"`
			Period               string                       `json:"period"`
			Currency             string                       `json:"currency"`
			TotalAmount          string                       `json:"totalAmount"`
			LineCount            int64                        `json:"lineCount"`
			AdjustmentCount      int64                        `json:"adjustmentCount"`
			PublishedAt          string                       `json:"publishedAt"`
			Disputes             []map[string]json.RawMessage `json:"disputes"`
			SubsequentInclusions []map[string]json.RawMessage `json:"subsequentInclusions"`
		} `json:"statements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CUSTOMER_STATEMENTS_LISTED" || len(body.Statements) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	statement := body.Statements[0]
	if statement.StatementNumber != "SYN-STMT-01" || statement.Account != "SYN-ACC-01" ||
		statement.Period != "2026-07" || statement.Currency != "SYN-CUR-01" ||
		statement.TotalAmount != "250000" ||
		statement.PublishedAt != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("单面转写走样：%+v", statement)
	}
	// 计数是行数不是金额：总额不由行数派生，两者各自照实转写。
	if statement.LineCount != 4 || statement.AdjustmentCount != 1 {
		t.Fatalf("行数与调整数走样：%+v", statement)
	}

	if len(statement.Disputes) != 2 {
		t.Fatalf("异议数走样：%s", response.Body.String())
	}
	resolved := statement.Disputes[0]
	if string(resolved["resolution"]) != `"UPHELD"` ||
		string(resolved["resolutionRef"]) != `"SYN-RES-01"` ||
		string(resolved["resolvedAt"]) != `"`+resolvedAt.Format(time.RFC3339Nano)+`"` ||
		string(resolved["disputedAmount"]) != `"12000"` {
		t.Fatalf("已裁定异议转写走样：%s", response.Body.String())
	}
	// 争议号与无号原因恰出其一（ADR-0355 决定十一）：有号的不带原因；没号的不出空串、不拿争议标识顶，改带原因。
	if string(resolved["documentNumber"]) != `"DSP-202607-00003"` || resolved["documentNumberAbsence"] != nil {
		t.Fatalf("有争议号的那一项转写走样：%s", response.Body.String())
	}
	open := statement.Disputes[1]
	if open["documentNumber"] != nil || string(open["documentNumberAbsence"]) != `"ISSUED_BEFORE_NUMBERING"` ||
		string(open["dispute"]) != `"SYN-DISP-OPEN"` {
		t.Fatalf("没有争议号的那一项转写走样：%s", response.Body.String())
	}
	for _, absent := range []string{"resolution", "resolutionRef", "resolvedAt"} {
		if _, present := open[absent]; present {
			t.Fatalf("未裁定的异议不该带 %q（那会写成一个看起来已有人处置的结论）：%s",
				absent, response.Body.String())
		}
	}

	if len(statement.SubsequentInclusions) != 2 {
		t.Fatalf("纳入数走样：%s", response.Body.String())
	}
	adjustment := statement.SubsequentInclusions[0]
	if string(adjustment["kind"]) != `"ADJUSTMENT"` ||
		string(adjustment["adjustment"]) != `"SYN-ADJ-01"` ||
		string(adjustment["originalPeriod"]) != `"2026-06"` ||
		string(adjustment["subsequentPeriod"]) != `"2026-07"` {
		t.Fatalf("调整类纳入转写走样：%s", response.Body.String())
	}
	late := statement.SubsequentInclusions[1]
	if _, present := late["adjustment"]; present {
		t.Fatalf("迟到费用纳入不该指名调整（迁移 0007）：%s", response.Body.String())
	}
	// 纳入只拥有关系，金额永远在费用或调整本体上。
	for _, entry := range statement.SubsequentInclusions {
		for _, absent := range []string{"amount", "includedAmount"} {
			if _, present := entry[absent]; present {
				t.Fatalf("纳入行长出了金额键 %q：%s", absent, response.Body.String())
			}
		}
	}
}

// Covers: 作废是留痕不是消失 — 已作废单仍在册，总额与两个计数原样保留，作废两键成对
// 在场；未作废单两键成对缺席。
func TestCustomerStatementQueryKeepsVoidedStatementsInPlace(t *testing.T) {
	voidedAt := catalogueBaseAt.Add(96 * time.Hour)
	register := &stubStatementCatalogue{
		statements: []ports.CustomerStatementCatalogueRow{
			{
				StatementNumber: "SYN-STMT-VOID",
				Account:         "SYN-ACC-01",
				Period:          "2026-06",
				Currency:        "SYN-CUR-01",
				TotalMinor:      180000,
				LineCount:       3,
				PublishedAt:     catalogueBaseAt,
				VoidBasis:       "SYN-VOID-BASIS-01",
				VoidedAt:        &voidedAt,
			},
			{
				StatementNumber: "SYN-STMT-LIVE",
				Account:         "SYN-ACC-01",
				Period:          "2026-07",
				Currency:        "SYN-CUR-01",
				TotalMinor:      250000,
				LineCount:       4,
				PublishedAt:     catalogueBaseAt,
			},
		},
	}
	response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=customer-statement")

	var body struct {
		Statements []map[string]json.RawMessage `json:"statements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if len(body.Statements) != 2 {
		t.Fatalf("作废单不在册：%s", response.Body.String())
	}
	voided := body.Statements[0]
	if string(voided["voidBasis"]) != `"SYN-VOID-BASIS-01"` ||
		string(voided["voidedAt"]) != `"`+voidedAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("作废留痕走样：%s", response.Body.String())
	}
	// 作废不删行不改总额：内容原样留在这一行上。
	if string(voided["totalAmount"]) != `"180000"` || string(voided["lineCount"]) != "3" {
		t.Fatalf("作废单的内容被动过：%s", response.Body.String())
	}
	live := body.Statements[1]
	for _, absent := range []string{"voidBasis", "voidedAt"} {
		if _, present := live[absent]; present {
			t.Fatalf("未作废单不该带 %q：%s", absent, response.Body.String())
		}
	}
}

// Covers: 逐格转写 — 主张身份与版本、行数与匹配数原样透出；审核授权是否已配置照实
// 转写，不折成「可审核」（UC-SA-004：授权未配置时审核停在未决）。
func TestSupplierBillReceptionQueryTranscribesCountsAndAuthority(t *testing.T) {
	register := &stubStatementCatalogue{
		receptions: []ports.SupplierBillReceptionCatalogueRow{
			{
				Claim:                    "SYN-CLAIM-01",
				ClaimVersion:             "V2",
				Supplier:                 "SYN-SUP-01",
				LegalEntity:              "SYN-LE-01",
				Period:                   "2026-07",
				Currency:                 "SYN-CUR-01",
				LineCount:                12,
				MatchCount:               9,
				AuditAuthorityConfigured: true,
				RecordedAt:               catalogueBaseAt,
			},
			{
				Claim:        "SYN-CLAIM-02",
				ClaimVersion: "V1",
				Supplier:     "SYN-SUP-02",
				LegalEntity:  "SYN-LE-01",
				Period:       "2026-07",
				Currency:     "SYN-CUR-01",
				RecordedAt:   catalogueBaseAt,
			},
		},
	}
	response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=supplier-bill-reception")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome    string `json:"outcome"`
		Receptions []struct {
			Claim                    string `json:"claim"`
			ClaimVersion             string `json:"claimVersion"`
			Supplier                 string `json:"supplier"`
			LegalEntity              string `json:"legalEntity"`
			Period                   string `json:"period"`
			Currency                 string `json:"currency"`
			LineCount                int64  `json:"lineCount"`
			MatchCount               int64  `json:"matchCount"`
			AuditAuthorityConfigured bool   `json:"auditAuthorityConfigured"`
			RecordedAt               string `json:"recordedAt"`
		} `json:"receptions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "SUPPLIER_BILL_RECEPTIONS_LISTED" || len(body.Receptions) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	audited := body.Receptions[0]
	if audited.Claim != "SYN-CLAIM-01" || audited.ClaimVersion != "V2" ||
		audited.Supplier != "SYN-SUP-01" || audited.LegalEntity != "SYN-LE-01" ||
		audited.Period != "2026-07" || audited.Currency != "SYN-CUR-01" ||
		audited.RecordedAt != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("主张面转写走样：%+v", audited)
	}
	// 未匹配是行数之差，读面只上两个数：把它算成一个「未匹配数」就是替详情面作答。
	if audited.LineCount != 12 || audited.MatchCount != 9 || !audited.AuditAuthorityConfigured {
		t.Fatalf("行数、匹配数或授权位走样：%+v", audited)
	}
	// 一行都没有与授权未配置分别在场，两者都不缺席（bool 不带 omitempty）。
	unconfigured := body.Receptions[1]
	if unconfigured.LineCount != 0 || unconfigured.MatchCount != 0 || unconfigured.AuditAuthorityConfigured {
		t.Fatalf("空主张的三个格走样：%+v", unconfigured)
	}
	if !json.Valid(response.Body.Bytes()) {
		t.Fatalf("答复不是合法 JSON：%s", response.Body.String())
	}
}

// Covers: AT-SA-232 — 账单册的汇总逐格透出：主张金额之和、按匹配分类原名计的行数（没有的分类没有键，空也答 {} 不答 null）、
// 已形成应付的行数与金额、已匹配而没有应付的行数、到达时刻；被追加的原主张与接收操作者缺席即不出键。AT-SA-233、AT-SA-234：
// 已拒绝、仍在争议中、还没有结果三个和与供应商账单号照实透出，没有账单号不出键。
func TestSupplierBillReceptionQueryCarriesTheBillSummary(t *testing.T) {
	register := &stubStatementCatalogue{
		receptions: []ports.SupplierBillReceptionCatalogueRow{
			{
				Claim: "SYN-SUP-01/INV-1", ClaimVersion: "1", Supplier: "SYN-SUP-01", LegalEntity: "SYN-LE-01", Period: "2026-08",
				Currency: "CNY", LineCount: 3, MatchCount: 3, TotalClaimedMinor: 3300,
				ClassificationCounts: map[string]int64{"MATCHED": 2, "PRICE_VARIANCE": 1},
				PayableLineCount:     1, PayableMinor: 1000, MatchedWithoutPayableCount: 1,
				RejectedMinor: 300, OpenDisputeMinor: 200, UndecidedMinor: 1800, SupplierBillNumber: "INV-1",
				AuditAuthorityConfigured: false, AuditAuthorityConfiguredNow: true,
				ReceivedAt: catalogueBaseAt.Add(-time.Hour), Supplements: "SYN-SUP-01/INV-0", OperationEvidence: "OPERATOR/syn#clerk",
				RecordedAt: catalogueBaseAt,
			},
			{Claim: "SYN-SUP-01/INV-2", ClaimVersion: "1", Supplier: "SYN-SUP-01", LegalEntity: "SYN-LE-01", Period: "2026-08",
				Currency: "CNY", ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt},
		},
	}
	response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=supplier-bill-reception")
	var body struct {
		Receptions []map[string]json.RawMessage `json:"receptions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Receptions) != 2 {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	summary, bare := body.Receptions[0], body.Receptions[1]
	for key, want := range map[string]string{
		"totalClaimedAmount":         `"3300"`,
		"classificationCounts":       `{"MATCHED":2,"PRICE_VARIANCE":1}`,
		"payableLineCount":           `1`,
		"payableAmount":              `"1000"`,
		"matchedWithoutPayableCount": `1`,
		"rejectedAmount":             `"300"`,
		"openDisputeAmount":          `"200"`,
		"undecidedAmount":            `"1800"`,
		"supplierBillNumber":         `"INV-1"`,
		"receivedAt":                 `"` + catalogueBaseAt.Add(-time.Hour).Format(time.RFC3339Nano) + `"`,
		"supplementsClaim":           `"SYN-SUP-01/INV-0"`,
		"operationEvidence":          `"OPERATOR/syn#clerk"`,
		// 接收时没登、后来补登了：快照与现状各说各的，不合成一格。
		"auditAuthorityConfigured":    `false`,
		"auditAuthorityConfiguredNow": `true`,
	} {
		if got := string(summary[key]); got != want {
			t.Fatalf("%s = %s, want %s：%s", key, got, want, response.Body.String())
		}
	}
	if got := string(bare["classificationCounts"]); got != `{}` {
		t.Fatalf("没有匹配的分类计数要答 {}：%s", got)
	}
	for _, absent := range []string{"supplementsClaim", "operationEvidence", "supplierBillNumber"} {
		if _, present := bare[absent]; present {
			t.Fatalf("缺席的 %q 不该出键：%s", absent, response.Body.String())
		}
	}
}

// Covers: AT-SA-232 — 账单册超过一页时答 truncated，只列一页；不到一页答 false。truncated 与 page.next 同义（照票
// catalogue-read-pagination-rollout spec「已带计数与 truncated 的待办队列保留这两格」对 truncated 的改法），读口收到的就是
// Intake 的页大小，不再多要一行。
func TestSupplierBillReceptionQueryFlagsAPageCut(t *testing.T) {
	rows := func(count int) []ports.SupplierBillReceptionCatalogueRow {
		receptions := make([]ports.SupplierBillReceptionCatalogueRow, 0, count)
		for index := range count {
			receptions = append(receptions, ports.SupplierBillReceptionCatalogueRow{
				Claim: fmt.Sprintf("SYN-SUP-01/INV-%03d", index), ClaimVersion: "1", ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt,
			})
		}
		return receptions
	}
	for _, probe := range []struct {
		name      string
		stored    int
		listed    int
		truncated bool
	}{
		{"多于一页", 30, 25, true},
		{"正好一页", 25, 25, false},
	} {
		t.Run(probe.name, func(t *testing.T) {
			register := &stubStatementCatalogue{receptions: rows(probe.stored)}
			response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=supplier-bill-reception")
			var body struct {
				Truncated  *bool             `json:"truncated"`
				Receptions []json.RawMessage `json:"receptions"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
			}
			if register.gotLimit != 25 || body.Truncated == nil || *body.Truncated != probe.truncated || len(body.Receptions) != probe.listed {
				t.Fatalf("limit=%d truncated=%v listed=%d：%s", register.gotLimit, body.Truncated, len(body.Receptions), response.Body.String())
			}
			if next := string(pageOf(t, response)["next"]); (next != "null") != probe.truncated {
				t.Fatalf("truncated=%v 而 page.next=%s：两格该同义", probe.truncated, next)
			}
		})
	}
}

// Covers: ADR-0077 Decision 四 — 两本册子空册各自是 2xx + 空数组。
func TestEmptyStatementRegistersAnswerEmptyArrays(t *testing.T) {
	endpoint := statementsEndpoint(&stubStatementCatalogue{})
	for registry, key := range map[string]string{
		"customer-statement":      "statements",
		"supplier-bill-reception": "receptions",
	} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-statements?registry="+registry)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if got := arrayAt(t, response, key); got != "[]" {
				t.Fatalf("空册没有以空数组在场：%s", got)
			}
		})
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestFailingStatementReadsAreNoAnswerRatherThanEmptyRegisters(t *testing.T) {
	endpoint := statementsEndpoint(&stubStatementCatalogue{err: errors.New("connection refused")})
	for _, registry := range []string{"customer-statement", "supplier-bill-reception"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-statements?registry="+registry)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
			}
			if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
				t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: AT-SA-233（端点半边）——账单册收 claim、supplier、supplierBillNumber 三个精确检索维，逐字转给读口；不给就是空检索。
func TestSupplierBillReceptionQueryPassesTheExactLookup(t *testing.T) {
	register := &stubStatementCatalogue{}
	serveGet(statementsEndpoint(register), "/settlement-statements?registry=supplier-bill-reception&supplier=SYN-SUP-01&supplierBillNumber=INV%201&claim=BILL-1")
	if register.gotReceptionFilter != (ports.SupplierBillReceptionFilter{Claim: "BILL-1", Supplier: "SYN-SUP-01", SupplierBillNumber: "INV 1"}) {
		t.Fatalf("检索维 = %+v", register.gotReceptionFilter)
	}
	serveGet(statementsEndpoint(register), "/settlement-statements?registry=supplier-bill-reception")
	if register.gotReceptionFilter != (ports.SupplierBillReceptionFilter{}) {
		t.Fatalf("不给检索维 = %+v", register.gotReceptionFilter)
	}
}
