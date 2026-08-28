package collectionhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

var catalogueBaseAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面:作用域与页大小都来自它,不读
// 请求内容。真通道未登记(PAR-INT-01),生产装配点不会有这样的实现——它只在测试里
// 存在,为的是隔离验证端点的转写。
type grantedCatalogueIntake struct {
	tenant string
	limit  int
}

func (intake grantedCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (collectionhttp.CatalogueQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return collectionhttp.CatalogueQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return collectionhttp.CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return collectionhttp.CatalogueQuery{}, err
	}
	return collectionhttp.CatalogueQuery{Scope: scope, Limit: intake.limit}, nil
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (collectionhttp.CatalogueQuery, error) {
	return collectionhttp.CatalogueQuery{}, intake.err
}

// stubCodSubledgers 是读口替身:记录收到的键,交回预置的册子或故障。
type stubCodSubledgers struct {
	entries []ports.CodSubledgerCatalogueRow
	err     error

	gotTenant string
	gotLimit  int
}

func (stub *stubCodSubledgers) ListCodSubledgers(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CodSubledgerCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.entries, stub.err
}

// unreachableCodSubledgers 断言读口未被触到:传输形状的拒绝与未配置格都发生在读库
// 之前。
type unreachableCodSubledgers struct{ t *testing.T }

func (stub unreachableCodSubledgers) ListCodSubledgers(
	context.Context, domain.TenantID, int,
) ([]ports.CodSubledgerCatalogueRow, error) {
	stub.t.Fatal("a refused request reached the subledger register")
	return nil, nil
}

func subledgerRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/collection-subledgers"+query, nil)
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	return body.Error.Code
}

func assertNoOutcome(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("拒绝答复不该带业务结果格:%s", response.Body.String())
	}
}

// Covers: ADR-0022 — 方法不对不是业务答案:405 + Allow,读口不被触到。
func TestCodSubledgerQueryRefusesNonGetMethods(t *testing.T) {
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 50},
		unreachableCodSubledgers{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/collection-subledgers", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 一律 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED,读口不被触到,不带业务结果格。
func TestUnconfiguredCodSubledgerIntakeRefusesEveryRequest(t *testing.T) {
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		collectionhttp.UnconfiguredIntake{}, unreachableCodSubledgers{t: t},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, subledgerRequest(t, ""))

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: 逐格转写 — 四维键、保管依据、开立时间、余额最小单位计数串、记账笔数与
// 批次数组原样透出;租户与页大小从作用域来,不采信请求自报;空批次以空数组在场。
func TestCodSubledgerListTranscribesLedgersVerbatim(t *testing.T) {
	register := &stubCodSubledgers{
		entries: []ports.CodSubledgerCatalogueRow{
			{
				Customer:     "SYN-CUST-01",
				LegalEntity:  "SYN-LE-01",
				Currency:     "SYN-CUR-01",
				Channel:      "SYN-CHAN-01",
				CustodyBasis: "SYN-CUSTODY-01",
				OpenedAt:     catalogueBaseAt,
				Balances: ports.CodSubledgerPositionBalances{
					AwaitingAllocationMinor: 50000,
					PayableToCustomerMinor:  100000,
				},
				PostingCount: 3,
				Batches: []ports.CodSubledgerRemittanceBatch{
					{
						Batch:            "SYN-BATCH-01",
						State:            "COLLECTED",
						CollectedThrough: catalogueBaseAt.Add(72 * time.Hour),
						FormedAt:         catalogueBaseAt,
					},
				},
			},
			{
				Customer:     "SYN-CUST-02",
				LegalEntity:  "SYN-LE-01",
				Currency:     "SYN-CUR-01",
				Channel:      "SYN-CHAN-01",
				CustodyBasis: "SYN-CUSTODY-01",
				OpenedAt:     catalogueBaseAt,
				Batches:      nil,
			},
		},
	}
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, subledgerRequest(t, "?tenant=TENANT-9&limit=9999"))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样:tenant=%q limit=%d(自报查询参数被采信了)",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome    string `json:"outcome"`
		Subledgers []struct {
			Customer     string `json:"customer"`
			LegalEntity  string `json:"legalEntity"`
			Currency     string `json:"currency"`
			Channel      string `json:"channel"`
			CustodyBasis string `json:"custodyBasis"`
			OpenedAt     string `json:"openedAt"`
			PostingCount int64  `json:"postingCount"`
			Balances     struct {
				InTransitAtChannel string `json:"inTransitAtChannel"`
				AwaitingAllocation string `json:"awaitingAllocation"`
				PayableToCustomer  string `json:"payableToCustomer"`
				Remitted           string `json:"remitted"`
				Shortfall          string `json:"shortfall"`
				Surplus            string `json:"surplus"`
			} `json:"balances"`
			RemittanceBatches json.RawMessage `json:"remittanceBatches"`
		} `json:"subledgers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "COD_SUBLEDGERS_LISTED" || len(body.Subledgers) != 2 {
		t.Fatalf("响应走样:%s", response.Body.String())
	}
	posted := body.Subledgers[0]
	if posted.Customer != "SYN-CUST-01" || posted.LegalEntity != "SYN-LE-01" ||
		posted.Currency != "SYN-CUR-01" || posted.Channel != "SYN-CHAN-01" ||
		posted.CustodyBasis != "SYN-CUSTODY-01" ||
		posted.OpenedAt != catalogueBaseAt.Format(time.RFC3339Nano) ||
		posted.PostingCount != 3 {
		t.Fatalf("开立面转写走样:%+v", posted)
	}
	if posted.Balances.AwaitingAllocation != "50000" ||
		posted.Balances.PayableToCustomer != "100000" ||
		posted.Balances.InTransitAtChannel != "0" ||
		posted.Balances.Remitted != "0" ||
		posted.Balances.Shortfall != "0" ||
		posted.Balances.Surplus != "0" {
		t.Fatalf("余额没有以最小单位计数串在场:%+v", posted.Balances)
	}
	var batches []struct {
		Batch            string `json:"batch"`
		State            string `json:"state"`
		CollectedThrough string `json:"collectedThrough"`
		FormedAt         string `json:"formedAt"`
	}
	if err := json.Unmarshal(posted.RemittanceBatches, &batches); err != nil {
		t.Fatalf("decode batches %s: %v", posted.RemittanceBatches, err)
	}
	if len(batches) != 1 || batches[0].Batch != "SYN-BATCH-01" || batches[0].State != "COLLECTED" ||
		batches[0].CollectedThrough != catalogueBaseAt.Add(72*time.Hour).Format(time.RFC3339Nano) ||
		batches[0].FormedAt != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("批次转写走样:%+v", batches)
	}
	// 尚无批次的账以空数组在场而不是 null:「实例半边未配置」这句话由页面对空数组
	// 说,传输层先保证空数组本身在场。
	if unposted := body.Subledgers[1]; string(unposted.RemittanceBatches) != "[]" {
		t.Fatalf("空批次没有以空数组在场:%s", unposted.RemittanceBatches)
	}
}

// Covers: ADR-0077 Decision 四 — 空册是 2xx 成格 + 空数组(不是 null):空册的续办
// 是去登记口开立,不是配置渠道,两格不得合并。
func TestAnEmptyCodSubledgerRegisterAnswersAnEmptyArray(t *testing.T) {
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}, &stubCodSubledgers{},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, subledgerRequest(t, ""))

	var fields struct {
		Subledgers json.RawMessage `json:"subledgers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if response.Code != http.StatusOK || string(fields.Subledgers) != "[]" {
		t.Fatalf("空册没有以空数组在场:%s", response.Body.String())
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成(5xx),不伪装成空册:前者该重试,
// 后者是终局答案。
func TestAFailingCodSubledgerReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		grantedCatalogueIntake{tenant: "TENANT-1", limit: 25},
		&stubCodSubledgers{err: errors.New("connection refused")},
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, subledgerRequest(t, ""))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: ADR-0022 — Intake 的三格各自映射:坏请求 400、未配置 403(上面单独用例)、
// 其余是 500 INTAKE_FAILED。
func TestCatalogueIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := collectionhttp.NewQueryCodSubledgersEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", collectionhttp.ErrMalformedRequest)},
		unreachableCodSubledgers{t: t},
	)
	response := httptest.NewRecorder()
	malformed.ServeHTTP(response, subledgerRequest(t, ""))
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}

	failing := collectionhttp.NewQueryCodSubledgersEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableCodSubledgers{t: t},
	)
	response = httptest.NewRecorder()
	failing.ServeHTTP(response, subledgerRequest(t, ""))
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
}
