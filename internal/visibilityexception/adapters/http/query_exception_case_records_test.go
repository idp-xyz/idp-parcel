package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// caseReaderDouble 按注入行作答，并记录收到的租户与页大小。
type caseReaderDouble struct {
	tenant domain.TenantID
	limit  int

	cases []ports.ExceptionCaseCatalogueRow
	err   error
}

func (double *caseReaderDouble) ListExceptionCases(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ExceptionCaseCatalogueRow, error) {
	double.tenant = tenant
	double.limit = limit
	if double.err != nil {
		return nil, double.err
	}
	return double.cases, nil
}

// unreachableCaseReader 是「被调即失败」的替身。
type unreachableCaseReader struct{ t *testing.T }

func (reader unreachableCaseReader) ListExceptionCases(
	context.Context, domain.TenantID, int,
) ([]ports.ExceptionCaseCatalogueRow, error) {
	reader.t.Error("a request passed the unconfigured intake and reached the case reader")
	return nil, nil
}

func ecServe(
	t *testing.T,
	intake visibilityhttp.OperationsTrackingIntake,
	reader visibilityhttp.CaseReviewReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewQueryExceptionCaseRecordsEndpoint(intake, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// Covers: 方法检查与未配置准入——单册端点没有 registry 轴，GET 之外 405，未配置
// Intake 答 403 且读口不被触到，4xx 不带 outcome。
func TestExceptionCaseRecordsRefuseWrongMethodAndUnconfiguredIntake(t *testing.T) {
	wrongMethod := ecServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&caseReaderDouble{}, http.MethodPost, "/exception-case-records")
	if wrongMethod.Code != http.StatusMethodNotAllowed ||
		wrongMethod.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405/GET",
			wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}

	unconfigured := ecServe(t, visibilityhttp.UnconfiguredIntake{},
		unreachableCaseReader{t: t}, http.MethodGet, "/exception-case-records")
	if unconfigured.Code != http.StatusForbidden ||
		problemCode(t, unconfigured) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s，want 403 ACCESS_CHANNEL_NOT_CONFIGURED",
			unconfigured.Code, unconfigured.Body.String())
	}
	assertNoOutcome(t, unconfigured)
}

// Covers: 案件册 200 + 行体逐字段透出——已关闭并归并的行带结论/关闭/归并键，开立行
// 三键缺席（工作条件、严重度、优先级结构上不存在，缺席即答案）；作用域与页大小来自
// Intake 裁决；空册交回空数组。
func TestExceptionCaseRecordsListVerbatim(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	firstResponse := operationsBaseAt.Add(time.Hour)
	closedAt := operationsBaseAt.Add(72 * time.Hour)
	reader := &caseReaderDouble{
		cases: []ports.ExceptionCaseCatalogueRow{
			{
				CaseID:          "case-1",
				RootParcel:      "parcel-1",
				ImpactScope:     "SINGLE_PARCEL",
				ResponsibleTeam: "ops-team-1",
				Phase:           "CLOSED",
				EstablishedAt:   operationsBaseAt,
				FirstResponse:   &firstResponse,
				ClosedAt:        &closedAt,
				Conclusion:      "RESOLVED_DELIVERED",
				MergedInto:      "case-9",
			},
			{
				CaseID:          "case-2",
				RootParcel:      "parcel-2",
				ImpactScope:     "BATCH",
				ResponsibleTeam: "ops-team-2",
				Phase:           "AWAITING_RESPONSE",
				EstablishedAt:   operationsBaseAt,
			},
		},
	}

	response := ecServe(t, intake, reader, http.MethodGet, "/exception-case-records")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Cases   []struct {
			CaseID          string `json:"caseId"`
			RootParcel      string `json:"rootParcel"`
			ImpactScope     string `json:"impactScope"`
			ResponsibleTeam string `json:"responsibleTeam"`
			Phase           string `json:"phase"`
			EstablishedAt   string `json:"establishedAt"`
			FirstResponse   any    `json:"firstResponse"`
			ClosedAt        any    `json:"closedAt"`
			Conclusion      any    `json:"conclusion"`
			MergedInto      any    `json:"mergedInto"`
			Severity        any    `json:"severity"`
			Priority        any    `json:"priority"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "EXCEPTION_CASES_LISTED" || len(body.Cases) != 2 {
		t.Fatalf("outcome = %q cases = %d", body.Outcome, len(body.Cases))
	}
	closed := body.Cases[0]
	if closed.CaseID != "case-1" || closed.ImpactScope != "SINGLE_PARCEL" ||
		closed.Phase != "CLOSED" ||
		closed.EstablishedAt != operationsBaseAt.Format(time.RFC3339Nano) ||
		closed.FirstResponse != firstResponse.Format(time.RFC3339Nano) ||
		closed.ClosedAt != closedAt.Format(time.RFC3339Nano) ||
		closed.Conclusion != "RESOLVED_DELIVERED" || closed.MergedInto != "case-9" {
		t.Fatalf("已关闭行走样：%+v", closed)
	}
	open := body.Cases[1]
	if open.CaseID != "case-2" || open.Phase != "AWAITING_RESPONSE" ||
		open.FirstResponse != nil || open.ClosedAt != nil ||
		open.Conclusion != nil || open.MergedInto != nil {
		t.Fatalf("开立行不得带关闭键：%+v", open)
	}
	// 页面模板的严重度/优先级在存储上没有登记格：键结构上不存在，不是空值。
	if closed.Severity != nil || closed.Priority != nil {
		t.Fatalf("行体不得代填严重度/优先级：%+v", closed)
	}
	if reader.tenant.String() != "tenant-1" || reader.limit != 50 {
		t.Fatalf("读口收到 tenant=%q limit=%d，应来自 Intake 裁决（tenant-1/50）",
			reader.tenant, reader.limit)
	}

	empty := ecServe(t, intake, &caseReaderDouble{}, http.MethodGet, "/exception-case-records")
	var emptyBody struct {
		Cases []json.RawMessage `json:"cases"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if empty.Code != http.StatusOK || emptyBody.Cases == nil {
		t.Fatalf("空册 = %d %s；want 200 + 空数组", empty.Code, empty.Body.String())
	}
}

// Covers: 读不回是答案未形成（500 + NO_ANSWER_FORMED），不伪装成空册。
func TestExceptionCaseRecordsReaderFailureFormsNoAnswer(t *testing.T) {
	response := ecServe(t, operationsIntakeDouble{query: operationsScope(t)},
		&caseReaderDouble{err: context.DeadlineExceeded},
		http.MethodGet, "/exception-case-records")
	if response.Code != http.StatusInternalServerError ||
		problemCode(t, response) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s，want 500 NO_ANSWER_FORMED",
			response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}
