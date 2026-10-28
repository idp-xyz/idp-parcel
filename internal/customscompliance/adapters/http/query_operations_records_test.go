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
