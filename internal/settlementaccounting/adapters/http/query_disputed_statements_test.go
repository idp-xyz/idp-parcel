package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type stubDisputedStatements struct {
	listed ports.DisputedCustomerStatements
	err    error

	gotTenant string
	gotLimit  int
}

func (stub *stubDisputedStatements) ListDisputedCustomerStatements(
	_ context.Context, tenant domain.TenantID, limit int,
) (ports.DisputedCustomerStatements, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.listed, stub.err
}

func serveDisputed(t *testing.T, stub *stubDisputedStatements, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	settlementhttp.NewQueryDisputedCustomerStatementsEndpoint(grantedIntake(), stub).
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

// Covers: AT-SA-227 — 张数是读口答的全部张数，不是这一页的行数；页比张数短即标截断；行形与对账单册同一份；
// 租户与页大小只取自接入面，请求自报的租户不采信。
func TestDisputedStatementQueryCarriesTheWholeCountAndMarksTruncation(t *testing.T) {
	stub := &stubDisputedStatements{listed: ports.DisputedCustomerStatements{
		Count: 3,
		Statements: []ports.CustomerStatementCatalogueRow{{
			StatementNumber: "SYN-STMT-01", Account: "SYN-ACC-01", Period: "2026-07", Currency: "SYN-CUR-01",
			TotalMinor: 250000, LineCount: 2, PublishedAt: catalogueBaseAt,
			Disputes: []ports.StatementDisputeEntry{{
				Dispute: "SYN-DSP-01", Charge: "SYN-CHG-01", DisputedMinor: 1200, Reason: "SYN-RSN-01", OpenedAt: catalogueBaseAt,
			}},
		}},
	}}
	response := serveDisputed(t, stub, "/settlement-disputed-statements?tenant=TENANT-9")
	if response.Code != http.StatusOK || stub.gotTenant != "TENANT-1" || stub.gotLimit != 25 {
		t.Fatalf("status = %d tenant = %q limit = %d", response.Code, stub.gotTenant, stub.gotLimit)
	}
	var body struct {
		Outcome       string `json:"outcome"`
		DisputedCount int    `json:"disputedCount"`
		Truncated     bool   `json:"truncated"`
		Statements    []struct {
			StatementNumber string `json:"statementNumber"`
			TotalAmount     string `json:"totalAmount"`
			Disputes        []struct {
				Dispute        string `json:"dispute"`
				DisputedAmount string `json:"disputedAmount"`
				Resolution     string `json:"resolution"`
			} `json:"disputes"`
		} `json:"statements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "DISPUTED_CUSTOMER_STATEMENTS_LISTED" || body.DisputedCount != 3 || !body.Truncated {
		t.Fatalf("张数或截断走样（张数要的是全部，不是这一页）：%s", response.Body.String())
	}
	if len(body.Statements) != 1 || body.Statements[0].StatementNumber != "SYN-STMT-01" ||
		body.Statements[0].TotalAmount != "250000" || len(body.Statements[0].Disputes) != 1 ||
		body.Statements[0].Disputes[0].DisputedAmount != "1200" || body.Statements[0].Disputes[0].Resolution != "" {
		t.Fatalf("行形与对账单册不同：%s", response.Body.String())
	}
}

// Covers: AT-SA-227 — 没有挂着未了结争议的单答 0 张与空数组，不截断；读不回答 NO_ANSWER_FORMED，不伪装成 0 张。
func TestDisputedStatementQueryAnswersZeroOnlyWhenTheReadAnswersZero(t *testing.T) {
	response := serveDisputed(t, &stubDisputedStatements{}, "/settlement-disputed-statements")
	var body struct {
		DisputedCount int             `json:"disputedCount"`
		Truncated     bool            `json:"truncated"`
		Statements    json.RawMessage `json:"statements"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.DisputedCount != 0 || body.Truncated || string(body.Statements) != "[]" {
		t.Fatalf("空答案走样：%d %s", response.Code, response.Body.String())
	}

	failing := serveDisputed(t, &stubDisputedStatements{err: errors.New("down")}, "/settlement-disputed-statements")
	if failing.Code != http.StatusInternalServerError || problemCode(t, failing) != "NO_ANSWER_FORMED" {
		t.Fatalf("读不回必须答未形成：%d %s", failing.Code, failing.Body.String())
	}
}

type stubStatementLines struct {
	found     ports.StatementLines
	has       bool
	err       error
	gotNumber string
}

func (stub *stubStatementLines) FindStatementLines(_ context.Context, _ domain.TenantID, number string) (ports.StatementLines, bool, error) {
	stub.gotNumber = number
	return stub.found, stub.has, stub.err
}

// Covers: AT-SA-228（读口半边）——按单号读一张单的费用行：金额照最小币单位整数串转写；单不在册答 STATEMENT_NOT_FOUND（200）；缺单号
// 是坏请求；读不回答未形成。
func TestStatementLinesQueryAnswersTheStatementsOwnLines(t *testing.T) {
	stub := &stubStatementLines{has: true, found: ports.StatementLines{
		StatementNumber: "SYN-STMT-01", Currency: "CNY", PublishedAt: catalogueBaseAt,
		Lines: []ports.StatementLineEntry{{Charge: "SYN-CHG-01", AmountMinor: 9600}},
	}}
	serve := func(target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		settlementhttp.NewQueryStatementLinesEndpoint(grantedIntake(), stub).ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		return response
	}
	response := serve("/settlement-statement-lines?statement=SYN-STMT-01")
	var body struct {
		Outcome string `json:"outcome"`
		Lines   []struct {
			Charge string `json:"charge"`
			Amount string `json:"amount"`
		} `json:"lines"`
		VoidedAt string `json:"voidedAt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || stub.gotNumber != "SYN-STMT-01" || body.Outcome != "STATEMENT_LINES_LISTED" ||
		len(body.Lines) != 1 || body.Lines[0].Amount != "9600" || body.VoidedAt != "" {
		t.Fatalf("费用行走样：%d %s", response.Code, response.Body.String())
	}
	stub.has = false
	if missing := serve("/settlement-statement-lines?statement=SYN-STMT-09"); missing.Code != http.StatusOK || !strings.Contains(missing.Body.String(), "STATEMENT_NOT_FOUND") {
		t.Fatalf("不在册：%d %s", missing.Code, missing.Body.String())
	}
	if bad := serve("/settlement-statement-lines"); bad.Code != http.StatusBadRequest {
		t.Fatalf("缺单号：%d", bad.Code)
	}
	stub.err = errors.New("down")
	if failing := serve("/settlement-statement-lines?statement=SYN-STMT-01"); failing.Code != http.StatusInternalServerError {
		t.Fatalf("读不回：%d", failing.Code)
	}
}
