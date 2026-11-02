package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证供应商账单与应付页的两本册（票 document-numbering/11）：经接收口、审核口真登一份账单、审一行之后，
// 按特性登记处那一份 build 与 mount 装上的读口读回——应付单号出现在应付册、账单各行与账单册的汇总里。查阅接入面用一只只认租户的
// 认证替身：查阅授予的判法由 OperatorOperationsReadIntake 与 access-identity 各自的用例证，这里证的是读口装在哪、读的是哪两张表。

type supplierBillBooksReader struct{ tenant sadomain.TenantID }

func (reader supplierBillBooksReader) AuthenticateOperationsRead(context.Context, string) (sadomain.TenantID, error) {
	return reader.tenant, nil
}

func mountSettlementCatalogue(t *testing.T, fixture *settlementDecisionsFixture) map[string]http.Handler {
	t.Helper()
	intake, err := settlementhttp.NewOperatorOperationsReadIntake(supplierBillBooksReader{tenant: fixture.tenant})
	if err != nil {
		t.Fatal(err)
	}
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*settlementCatalogueWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: fixture.db},
			endpointChannels{operatorRegistries: operatorRegistryIntakes{settlementOperationsRead: intake}})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, endpoint := range mounted {
			endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	return endpoints
}

func getSettlementBook(t *testing.T, endpoints map[string]http.Handler, pattern, query string, into any) {
	t.Helper()
	handler := endpoints[pattern]
	if handler == nil {
		t.Fatalf("%s 没按登记处挂上", pattern)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, pattern+query, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s%s = %d %s", pattern, query, response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
		t.Fatalf("%s 答复不是 JSON：%s", pattern, response.Body.String())
	}
}

// Covers: AT-SA-232（生产装配半边）、AT-SA-231（读口半边）——经两口真登一份账单、审一行之后：审核应付册以应付单号列出这一笔、带
// 供应商与点审核的人，按应付单号精确检索只剩它；账单各行上那一行挂着同一个应付单号；账单册的汇总数到一行已形成应付、没有已匹配
// 而未审的行。
func TestTheSupplierBillBooksShowTheAuditedPayableNumber(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated {
		t.Fatalf("接收 = %d %v", status, answer)
	}
	status, answer := fixture.post(t, supplierBillAuditFace, billAuditor, supplierBillAuditBody)
	number, _ := answer["documentNumber"].(string)
	if status != http.StatusCreated || !payableNumberPattern.MatchString(number) {
		t.Fatalf("审核 = %d %v", status, answer)
	}
	books := mountSettlementCatalogue(t, fixture)

	type payableRow struct {
		Payable           string `json:"payable"`
		DocumentNumber    string `json:"documentNumber"`
		Supplier          string `json:"supplier"`
		Claim             string `json:"claim"`
		Line              string `json:"line"`
		Amount            string `json:"amount"`
		OperationEvidence string `json:"operationEvidence"`
	}
	var payables struct {
		Outcome  string       `json:"outcome"`
		Payables []payableRow `json:"payables"`
	}
	getSettlementBook(t, books, "/settlement-audited-payables", "", &payables)
	if payables.Outcome != "AUDITED_PAYABLES_LISTED" || len(payables.Payables) != 1 {
		t.Fatalf("应付册 = %+v", payables)
	}
	row := payables.Payables[0]
	if row.DocumentNumber != number || row.Payable != "SYN-PAYABLE-PSB22-1" || row.Supplier != "SYN-PARTY-CARRIER-TRUNK-01" ||
		row.Claim != "SYN-BILL-PSB22-1" || row.Line != "L1" || row.Amount != "8500" || row.OperationEvidence != operatorEvidenceOf(billAuditor) {
		t.Fatalf("应付册那一笔 = %+v，want 应付单号 %s", row, number)
	}
	getSettlementBook(t, books, "/settlement-audited-payables", "?documentNumber="+url.QueryEscape(number), &payables)
	if len(payables.Payables) != 1 {
		t.Fatalf("按应付单号检索 = %+v", payables)
	}
	getSettlementBook(t, books, "/settlement-audited-payables", "?documentNumber=AP-000000-00000", &payables)
	if len(payables.Payables) != 0 {
		t.Fatalf("按没有的应付单号检索要答空 = %+v", payables)
	}

	var lines struct {
		Outcome string `json:"outcome"`
		Lines   []struct {
			Line           string `json:"line"`
			Classification string `json:"classification"`
			Payable        *struct {
				DocumentNumber string `json:"documentNumber"`
			} `json:"payable"`
		} `json:"lines"`
	}
	getSettlementBook(t, books, "/settlement-supplier-bill-lines", "?claim=SYN-BILL-PSB22-1&version=v1", &lines)
	if lines.Outcome != "SUPPLIER_BILL_LINES_LISTED" || len(lines.Lines) != 1 || lines.Lines[0].Classification != "MATCHED" ||
		lines.Lines[0].Payable == nil || lines.Lines[0].Payable.DocumentNumber != number {
		t.Fatalf("账单各行 = %+v", lines)
	}

	var receptions struct {
		Receptions []struct {
			Claim                      string `json:"claim"`
			TotalClaimedAmount         string `json:"totalClaimedAmount"`
			PayableLineCount           int64  `json:"payableLineCount"`
			MatchedWithoutPayableCount int64  `json:"matchedWithoutPayableCount"`
			OperationEvidence          string `json:"operationEvidence"`
		} `json:"receptions"`
	}
	getSettlementBook(t, books, "/settlement-statements", "?registry=supplier-bill-reception", &receptions)
	if len(receptions.Receptions) != 1 || receptions.Receptions[0].TotalClaimedAmount != "8500" ||
		receptions.Receptions[0].PayableLineCount != 1 || receptions.Receptions[0].MatchedWithoutPayableCount != 0 ||
		receptions.Receptions[0].OperationEvidence != operatorEvidenceOf(billClerk) {
		t.Fatalf("账单册汇总 = %+v", receptions)
	}
}
