package customshttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

type stubCaseRecords struct {
	records   []ports.CustomsCaseRecord
	err       error
	gotTenant string
	gotFilter string
}

func (stub *stubCaseRecords) ListCustomsCases(
	_ context.Context, tenant domain.TenantID, caseID string, _ int,
) ([]ports.CustomsCaseRecord, error) {
	stub.gotTenant, stub.gotFilter = tenant.String(), caseID
	return stub.records, stub.err
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未授予（未配置 Intake）答 403，读口不被触到。
func TestUnconfiguredIntakeRefusesCustomsRecordQueries(t *testing.T) {
	for _, handler := range []http.Handler{
		customshttp.NewQueryCustomsCasesEndpoint(customshttp.UnconfiguredIntake{}, &stubCaseRecords{err: errors.New("reached")}),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-case-records", nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	}
}

// Covers: 非 GET 答 405 + Allow。
func TestCustomsRecordQueriesRefuseNonGet(t *testing.T) {
	handler := customshttp.NewQueryCustomsCasesEndpoint(grantedCatalogueIntake{tenant: "T1", limit: 5}, &stubCaseRecords{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs-case-records", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q", response.Code, response.Header().Get("Allow"))
	}
}

// Covers: 租户只取自信封（请求自报的租户不采信）；空册以空数组在场；过滤参数原样下传。
func TestCustomsCaseQueryTakesTenantFromEnvelopeAndAnswersEmptyArray(t *testing.T) {
	stub := &stubCaseRecords{}
	handler := customshttp.NewQueryCustomsCasesEndpoint(grantedCatalogueIntake{tenant: "T1", limit: 5}, stub)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-case-records?tenant=T9&case=CASE-1", nil))
	if response.Code != http.StatusOK || stub.gotTenant != "T1" || stub.gotFilter != "CASE-1" {
		t.Fatalf("status = %d tenant = %q filter = %q", response.Code, stub.gotTenant, stub.gotFilter)
	}
	var body struct {
		Outcome string          `json:"outcome"`
		Cases   json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "CUSTOMS_CASES_LISTED" || string(body.Cases) != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", response.Body.String())
	}
}

// Covers: ADR-0022 — 读不回是答案未形成（5xx），不伪装成空册。
func TestFailingCustomsCaseReadIsNoAnswer(t *testing.T) {
	handler := customshttp.NewQueryCustomsCasesEndpoint(
		grantedCatalogueIntake{tenant: "T1", limit: 5}, &stubCaseRecords{err: errors.New("down")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-case-records", nil))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0355 决定十一——案件读口带单据号；没有单据号的案件不带 documentNumber 这一格（不答空串、不拿案件标识顶），
// 另由 documentNumberAbsence 说是上线前开立还是建案时单据编号时区未配置。点名 `AT-CC-459`、`AT-CC-460`（读口半边）。
func TestCustomsCaseRecordsCarryTheDocumentNumberOrItsAbsence(t *testing.T) {
	stub := &stubCaseRecords{records: []ports.CustomsCaseRecord{
		{Case: "CASE-1", DocumentNumber: "CC-20261102-0001"},
		{Case: "CASE-2", DocumentNumberAbsence: "NUMBERING_TIME_ZONE_NOT_CONFIGURED"},
		{Case: "CASE-3", DocumentNumberAbsence: "ISSUED_BEFORE_NUMBERING"},
	}}
	handler := customshttp.NewQueryCustomsCasesEndpoint(grantedCatalogueIntake{tenant: "T1", limit: 5}, stub)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customs-case-records", nil))
	var body struct {
		Cases []map[string]any `json:"cases"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK || len(body.Cases) != 3 {
		t.Fatalf("%d %s %v", response.Code, response.Body.String(), err)
	}
	if body.Cases[0]["documentNumber"] != "CC-20261102-0001" {
		t.Fatalf("有号的案件：%v", body.Cases[0])
	}
	if _, absent := body.Cases[0]["documentNumberAbsence"]; absent {
		t.Fatalf("有号的案件带了无号情形：%v", body.Cases[0])
	}
	for i, want := range map[int]string{1: "NUMBERING_TIME_ZONE_NOT_CONFIGURED", 2: "ISSUED_BEFORE_NUMBERING"} {
		if _, present := body.Cases[i]["documentNumber"]; present || body.Cases[i]["documentNumberAbsence"] != want {
			t.Fatalf("无号的案件 %d：%v", i, body.Cases[i])
		}
	}
}
