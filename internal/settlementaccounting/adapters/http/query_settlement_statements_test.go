package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// stubStatementCatalogue 是对账页两本册子的读口替身。
type stubStatementCatalogue struct {
	statements []ports.CustomerStatementCatalogueRow
	receptions []ports.SupplierBillReceptionCatalogueRow
	err        error

	gotTenant string
	gotLimit  int
}

func (stub *stubStatementCatalogue) ListCustomerStatements(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CustomerStatementCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.statements, stub.err
}

func (stub *stubStatementCatalogue) ListSupplierBillReceptions(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.SupplierBillReceptionCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.receptions, stub.err
}

func statementsEndpoint(register *stubStatementCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), register)
}

// Covers: 逐格转写 — 单号、账户、周期、总额与两个计数原样透出；异议与后续纳入以各自
// 数组挂在单上；未裁定的异议不带裁定三键（不代填「待处理」）；纳入行没有金额键。
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
						Dispute:       "SYN-DISP-RESOLVED",
						Charge:        "SYN-CHG-01",
						DisputedMinor: 12000,
						Reason:        "WEIGHT_MISMATCH",
						OpenedAt:      catalogueBaseAt.Add(24 * time.Hour),
						Resolution:    "UPHELD",
						ResolutionRef: "SYN-RES-01",
						ResolvedAt:    &resolvedAt,
					},
					{
						Dispute:       "SYN-DISP-OPEN",
						Charge:        "SYN-CHG-02",
						DisputedMinor: 8000,
						Reason:        "RATE_DISPUTE",
						OpenedAt:      catalogueBaseAt.Add(24 * time.Hour),
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
	response := serveGet(statementsEndpoint(register), "/settlement-statements?registry=customer-statement&tenant=TENANT-9")

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
	open := statement.Disputes[1]
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
