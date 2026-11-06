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

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var catalogueBaseAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// grantedCatalogueIntake 装出「认证已就位」的接入面：作用域与页大小都来自它，不读
// 请求内容。生产上的真实现是 OperatorOperationsReadIntake；这一只只在测试里存在，为的是隔离验证端点的转写。
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
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
	stub.refuse("customer charge")
	return cataloguepage.Result[ports.CustomerChargeCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListSupplierExpectedCosts(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
	stub.refuse("supplier expected cost")
	return cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListCustomerStatements(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
	stub.refuse("customer statement")
	return cataloguepage.Result[ports.CustomerStatementCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListDisputedCustomerStatements(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (ports.DisputedCustomerStatements, error) {
	stub.refuse("disputed customer statement")
	return ports.DisputedCustomerStatements{}, nil
}

func (stub unreachableCatalogues) FindStatementLines(
	context.Context, domain.TenantID, string,
) (ports.StatementLines, bool, error) {
	stub.refuse("statement lines")
	return ports.StatementLines{}, false, nil
}

func (stub unreachableCatalogues) ListSupplierBillReceptions(
	context.Context, domain.TenantID, ports.SupplierBillReceptionFilter, int, cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
	stub.refuse("supplier bill reception")
	return cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) FindSupplierBillLines(
	context.Context, domain.TenantID, string, string,
) (ports.SupplierBillLines, bool, error) {
	stub.refuse("supplier bill lines")
	return ports.SupplierBillLines{}, false, nil
}

func (stub unreachableCatalogues) ListAuditedPayables(
	context.Context, domain.TenantID, ports.AuditedPayableFilter, int, cataloguepage.Query,
) (cataloguepage.Result[ports.AuditedPayableCatalogueRow], error) {
	stub.refuse("audited payable")
	return cataloguepage.Result[ports.AuditedPayableCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListExternalFundsFacts(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalFundsFactCatalogueRow], error) {
	stub.refuse("external funds fact")
	return cataloguepage.Result[ports.ExternalFundsFactCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListOperatingResults(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
	stub.refuse("operating result")
	return cataloguepage.Result[ports.OperatingResultCatalogueRow]{}, nil
}

func (stub unreachableCatalogues) ListCostAllocations(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.CostAllocationCatalogueRow], error) {
	stub.refuse("cost allocation")
	return cataloguepage.Result[ports.CostAllocationCatalogueRow]{}, nil
}

// endpointCase 是一个目录查阅端点：端点入口加一个能取到该册的合法目标。
type endpointCase struct {
	name    string
	target  string
	handler http.Handler
}

// refusedCatalogueEndpoints 交回本上下文的目录查阅端点，读口一律是断言未触到的替身。
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
			name:    "disputed-statements",
			target:  "/settlement-disputed-statements",
			handler: settlementhttp.NewQueryDisputedCustomerStatementsEndpoint(intake, unreachable),
		},
		{
			name:    "statement-lines",
			target:  "/settlement-statement-lines?statement=SYN-STMT-01",
			handler: settlementhttp.NewQueryStatementLinesEndpoint(intake, unreachable),
		},
		{
			name:    "supplier-bill-lines",
			target:  "/settlement-supplier-bill-lines?claim=SYN-SUP-01/INV-1&version=1",
			handler: settlementhttp.NewQuerySupplierBillLinesEndpoint(intake, unreachable),
		},
		{
			name:    "audited-payables",
			target:  "/settlement-audited-payables",
			handler: settlementhttp.NewQueryAuditedPayablesEndpoint(intake, unreachable),
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

// Covers: ADR-0055 决定三「答复对一切请求内容与自报身份一致，不因内容不同而变」——渠道未配置时，认不出的册名、空册名与合法册名
// 同答 403：分派排在 Intake 之后，没配好渠道的调用方从答复里探不出本端点认哪些册名。配好的渠道上认不出的册名照答 400，见
// TestSettlementCatalogueQueriesRejectUnknownRegistries。
func TestAnUnconfiguredChannelRefusesEvenAnUnknownRegistry(t *testing.T) {
	unreachable := unreachableCatalogues{t: t}
	dispatching := []struct {
		endpointCase
		known string
	}{
		{endpointCase{
			name:    "charges",
			target:  "/settlement-charges",
			handler: settlementhttp.NewQuerySettlementChargesEndpoint(settlementhttp.UnconfiguredIntake{}, unreachable),
		}, "customer-charge"},
		{endpointCase{
			name:    "statements",
			target:  "/settlement-statements",
			handler: settlementhttp.NewQuerySettlementStatementsEndpoint(settlementhttp.UnconfiguredIntake{}, unreachable),
		}, "customer-statement"},
		{endpointCase{
			name:    "operating-results",
			target:  "/settlement-operating-results",
			handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(settlementhttp.UnconfiguredIntake{}, unreachable),
		}, "operating-result"},
	}
	for _, endpoint := range dispatching {
		t.Run(endpoint.name, func(t *testing.T) {
			known := serveGet(endpoint.handler, endpoint.target+"?registry="+endpoint.known)
			if known.Code != http.StatusForbidden || problemCode(t, known) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("status = %d body = %s", known.Code, known.Body.String())
			}
			assertNoOutcome(t, known)
			for _, registry := range []string{"", "?registry=", "?registry=unknown", "?registry=customer_charge"} {
				response := serveGet(endpoint.handler, endpoint.target+registry)
				if response.Code != http.StatusForbidden || response.Body.String() != known.Body.String() {
					t.Fatalf("%q：未配置的渠道答复随册名变了：%d %s，合法册名答 %s", registry, response.Code, response.Body, known.Body)
				}
			}
		})
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
