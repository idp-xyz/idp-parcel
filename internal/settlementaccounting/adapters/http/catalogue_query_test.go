package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var catalogueBaseAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面：作用域与页大小都来自它，不读
// 请求内容。真渠道（操作者渠道，ADR-0100）未就位，生产装配点不会有这样的实现——它只在测试里
// 存在，为的是隔离验证端点的转写。
type grantedCatalogueIntake struct {
	tenant string
	limit  int
}

func (intake grantedCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (settlementhttp.CatalogueQuery, error) {
	reference, err := domain.NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		return settlementhttp.CatalogueQuery{}, err
	}
	tenant, err := domain.NewTenantID(intake.tenant)
	if err != nil {
		return settlementhttp.CatalogueQuery{}, err
	}
	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return settlementhttp.CatalogueQuery{}, err
	}
	return settlementhttp.CatalogueQuery{Scope: scope, Limit: intake.limit}, nil
}

func grantedIntake() grantedCatalogueIntake {
	return grantedCatalogueIntake{tenant: "TENANT-1", limit: 25}
}

type failingCatalogueIntake struct{ err error }

func (intake failingCatalogueIntake) IntakeCatalogueQuery(
	_ context.Context, _ *http.Request,
) (settlementhttp.CatalogueQuery, error) {
	return settlementhttp.CatalogueQuery{}, intake.err
}

// unreachableCatalogues 断言读口未被触到：传输形状的拒绝与未配置格都发生在读库之前。
// 一个类型实现四个读口——四个端点共用同一道分界，替身分设只会让它看起来能只守一半。
type unreachableCatalogues struct{ t *testing.T }

func (stub unreachableCatalogues) refuse(register string) {
	stub.t.Helper()
	stub.t.Fatalf("a refused request reached the %s register", register)
}

func (stub unreachableCatalogues) ListCustomerCharges(
	context.Context, domain.TenantID, int,
) ([]ports.CustomerChargeCatalogueRow, error) {
	stub.refuse("customer charge")
	return nil, nil
}

func (stub unreachableCatalogues) ListSupplierExpectedCosts(
	context.Context, domain.TenantID, int,
) ([]ports.SupplierExpectedCostCatalogueRow, error) {
	stub.refuse("supplier expected cost")
	return nil, nil
}

func (stub unreachableCatalogues) ListCustomerStatements(
	context.Context, domain.TenantID, int,
) ([]ports.CustomerStatementCatalogueRow, error) {
	stub.refuse("customer statement")
	return nil, nil
}

func (stub unreachableCatalogues) ListSupplierBillReceptions(
	context.Context, domain.TenantID, int,
) ([]ports.SupplierBillReceptionCatalogueRow, error) {
	stub.refuse("supplier bill reception")
	return nil, nil
}

func (stub unreachableCatalogues) ListExternalFundsFacts(
	context.Context, domain.TenantID, int,
) ([]ports.ExternalFundsFactCatalogueRow, error) {
	stub.refuse("external funds fact")
	return nil, nil
}

func (stub unreachableCatalogues) ListOperatingResults(
	context.Context, domain.TenantID, int,
) ([]ports.OperatingResultCatalogueRow, error) {
	stub.refuse("operating result")
	return nil, nil
}

func (stub unreachableCatalogues) ListCostAllocations(
	context.Context, domain.TenantID, int,
) ([]ports.CostAllocationCatalogueRow, error) {
	stub.refuse("cost allocation")
	return nil, nil
}

// endpointCase 是「四页七册」里的一格：端点入口加一个能取到该册的合法目标。
type endpointCase struct {
	name    string
	target  string
	handler http.Handler
}

// refusedCatalogueEndpoints 交回四个端点，读口一律是断言未触到的替身。
func refusedCatalogueEndpoints(t *testing.T, intake settlementhttp.CatalogueQueryIntake) []endpointCase {
	t.Helper()
	unreachable := unreachableCatalogues{t: t}
	return []endpointCase{
		{
			name:    "charges",
			target:  "/settlement-charges?registry=customer-charge",
			handler: settlementhttp.NewQuerySettlementChargesEndpoint(intake, unreachable),
		},
		{
			name:    "statements",
			target:  "/settlement-statements?registry=customer-statement",
			handler: settlementhttp.NewQuerySettlementStatementsEndpoint(intake, unreachable),
		},
		{
			name:    "funds-applications",
			target:  "/settlement-funds-applications",
			handler: settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(intake, unreachable),
		},
		{
			name:    "operating-results",
			target:  "/settlement-operating-results?registry=operating-result",
			handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(intake, unreachable),
		},
	}
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
		t.Fatalf("拒绝答复不该带业务结果格：%s", response.Body.String())
	}
}

// serveGet 打一份 GET 进端点，交回记录器。
func serveGet(handler http.Handler, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	return response
}

// arrayAt 取出答复里某个数组键的原文，用来分辨空数组与 null。
func arrayAt(t *testing.T, response *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	raw, present := fields[key]
	if !present {
		t.Fatalf("答复缺 %q 键：%s", key, response.Body.String())
	}
	return string(raw)
}

// Covers: ADR-0022 — 方法不对不是业务答案：405 + Allow，四个端点一致，读口不被触到。
func TestSettlementCatalogueQueriesRefuseNonGetMethods(t *testing.T) {
	for _, endpoint := range refusedCatalogueEndpoints(t, grantedIntake()) {
		t.Run(endpoint.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			endpoint.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint.target, nil))

			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
			}
			if allow := response.Header().Get("Allow"); allow != http.MethodGet {
				t.Fatalf("Allow = %q, want GET", allow)
			}
		})
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三 — 未配置 Intake 一律 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED，四个端点一致，读口不被触到，不带业务结果格。
func TestUnconfiguredIntakeRefusesEverySettlementCatalogueRequest(t *testing.T) {
	for _, endpoint := range refusedCatalogueEndpoints(t, settlementhttp.UnconfiguredIntake{}) {
		t.Run(endpoint.name, func(t *testing.T) {
			response := serveGet(endpoint.handler, endpoint.target)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", got)
			}
			assertNoOutcome(t, response)
		})
	}
}

// Covers: 分派参数的封闭集 — 未知册名、空册名都是坏请求（400），读口不被触到。
// 判据同 /customs-ports-paths：认不出要哪本册子时，答案不是随便挑一本。
func TestSettlementCatalogueQueriesRejectUnknownRegistries(t *testing.T) {
	unreachable := unreachableCatalogues{t: t}
	dispatching := []endpointCase{
		{
			name:    "charges",
			target:  "/settlement-charges",
			handler: settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), unreachable),
		},
		{
			name:    "statements",
			target:  "/settlement-statements",
			handler: settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), unreachable),
		},
		{
			name:    "operating-results",
			target:  "/settlement-operating-results",
			handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(grantedIntake(), unreachable),
		},
	}
	for _, endpoint := range dispatching {
		for _, registry := range []string{"", "?registry=", "?registry=unknown", "?registry=customer_charge"} {
			t.Run(endpoint.name+registry, func(t *testing.T) {
				response := serveGet(endpoint.handler, endpoint.target+registry)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d（%s）", response.Code, http.StatusBadRequest, response.Body.String())
				}
				if got := problemCode(t, response); got != "MALFORMED_REQUEST" {
					t.Fatalf("code = %q, want MALFORMED_REQUEST", got)
				}
				assertNoOutcome(t, response)
			})
		}
	}
}

// Covers: 分派参数的封闭集不跨端点通用 — 邻页的册名在本端点同样是未知值。三个端点
// 各自的封闭集互不通用，串台会把「问错了册子」答成一份看起来正常的答复。
func TestOneSettlementPageDoesNotAnswerAnotherPagesRegistry(t *testing.T) {
	unreachable := unreachableCatalogues{t: t}
	crossed := []endpointCase{
		{
			name:    "charges asked for an operating registry",
			target:  "/settlement-charges?registry=operating-result",
			handler: settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), unreachable),
		},
		{
			name:    "operating asked for a charge registry",
			target:  "/settlement-operating-results?registry=customer-charge",
			handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(grantedIntake(), unreachable),
		},
		{
			name:    "statements asked for a cost allocation registry",
			target:  "/settlement-statements?registry=cost-allocation",
			handler: settlementhttp.NewQuerySettlementStatementsEndpoint(grantedIntake(), unreachable),
		},
	}
	for _, endpoint := range crossed {
		t.Run(endpoint.name, func(t *testing.T) {
			response := serveGet(endpoint.handler, endpoint.target)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
		})
	}
}

// Covers: 门的次序 — 请求形状立不起来先于渠道未配置作答。册名认不出是请求自身的事，
// 判它不需要先知道调用方是谁；反过来先答 403 会让调用方以为配好渠道这次请求就能成。
func TestAnUnknownRegistryIsRefusedBeforeTheChannelCheck(t *testing.T) {
	endpoint := settlementhttp.NewQuerySettlementChargesEndpoint(
		settlementhttp.UnconfiguredIntake{}, unreachableCatalogues{t: t},
	)
	response := serveGet(endpoint, "/settlement-charges?registry=unknown")

	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0022 — Intake 的三格各自映射：坏请求 400、未配置 403（上面单独用例）、
// 其余是 500 INTAKE_FAILED。
func TestSettlementCatalogueIntakeFailuresKeepTheirGrades(t *testing.T) {
	malformed := settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(
		failingCatalogueIntake{err: fmt.Errorf("%w: no scope", settlementhttp.ErrMalformedRequest)},
		unreachableCatalogues{t: t},
	)
	response := serveGet(malformed, "/settlement-funds-applications")
	if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
		t.Fatalf("malformed: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)

	failing := settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(
		failingCatalogueIntake{err: errors.New("identity capability is down")},
		unreachableCatalogues{t: t},
	)
	response = serveGet(failing, "/settlement-funds-applications")
	if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
		t.Fatalf("failing: %d %s", response.Code, response.Body.String())
	}
	assertNoOutcome(t, response)
}
