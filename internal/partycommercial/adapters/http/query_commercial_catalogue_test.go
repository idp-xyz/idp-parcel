package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对两个目录端点(ADR-0077,票 master-data-wiring/05)证传输面:方法门、
// 未配置 Intake 一律 403、kind 封闭集先于 Intake、五种册子各自分派、行体逐字段
// 转写且可缺席字段如实缺席、空目录答空数组、读失败答 5xx。

var catBaseAt = time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)

func catValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type intakeDouble struct {
	query commercialhttp.CatalogueQuery
	err   error
}

func (double intakeDouble) IntakeCatalogueQuery(
	_ context.Context,
	_ *http.Request,
) (commercialhttp.CatalogueQuery, error) {
	if double.err != nil {
		return commercialhttp.CatalogueQuery{}, double.err
	}
	return double.query, nil
}

func catalogueQuery(t *testing.T) commercialhttp.CatalogueQuery {
	t.Helper()
	scope, err := domain.NewOperationsQueryScope(
		catValue(t, domain.NewOperationsScopeReference, "ops-scope-1"),
		catValue(t, domain.NewTenantID, "tenant-1"),
	)
	if err != nil {
		t.Fatalf("构造作用域:%v", err)
	}
	return commercialhttp.CatalogueQuery{Scope: scope, Limit: 50}
}

type productReaderDouble struct {
	tenant domain.TenantID
	rows   []ports.ServiceProductCatalogueRow
	err    error
}

func (double *productReaderDouble) ListServiceProducts(
	_ context.Context,
	tenant domain.TenantID,
	_ int,
) ([]ports.ServiceProductCatalogueRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

type policyReaderDouble struct {
	tenant      domain.TenantID
	packages    []ports.AcceptanceRulePackageRow
	controls    []ports.PreAcceptanceControlRow
	prices      []ports.PricePolicyRow
	settlements []ports.SettlementPolicyRow
	asOf        []ports.AsOfPolicyRow
	calls       map[string]int
	err         error
}

func (double *policyReaderDouble) record(method string) {
	if double.calls == nil {
		double.calls = map[string]int{}
	}
	double.calls[method]++
}

func (double *policyReaderDouble) ListAcceptanceRulePackages(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.AcceptanceRulePackageRow, error) {
	double.record("packages")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.packages, nil
}

func (double *policyReaderDouble) ListPreAcceptanceControls(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.PreAcceptanceControlRow, error) {
	double.record("controls")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.controls, nil
}

func (double *policyReaderDouble) ListPricePolicies(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.PricePolicyRow, error) {
	double.record("prices")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.prices, nil
}

func (double *policyReaderDouble) ListSettlementPolicies(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.SettlementPolicyRow, error) {
	double.record("settlements")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.settlements, nil
}

func (double *policyReaderDouble) ListAsOfPolicyDeclarations(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.AsOfPolicyRow, error) {
	double.record("asOf")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.asOf, nil
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON:%v\n%s", err, recorder.Body.String())
	}
	return body
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	body := decodeBody(t, recorder)
	detail, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("响应没有 error 体:%s", recorder.Body.String())
	}
	code, _ := detail["code"].(string)
	return code
}

func TestServiceProductsEndpointOnlyAcceptsGet(t *testing.T) {
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&productReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/commercial-service-products", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三——PAR-INT-01 未登记,生产装配的 Intake 对
// 每个请求一律 403,不读业务内容。
func TestServiceProductsEndpointAnswersUnconfiguredIntakeWith403(t *testing.T) {
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(
		commercialhttp.UnconfiguredIntake{},
		&productReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q", code)
	}
}

// Covers: 行体逐字段转写;形态与结束时刻可缺席,缺席时键如实不在场——目录不为齐整
// 补宽泛值(ADR-0050)。
func TestServiceProductsEndpointTranscribesRows(t *testing.T) {
	query := catalogueQuery(t)
	reader := &productReaderDouble{
		tenant: query.Scope.Tenant(),
		rows: []ports.ServiceProductCatalogueRow{
			{
				ObjectID:          "product-1",
				VersionLabel:      "v1",
				Scope:             "scope-1",
				Status:            "EFFECTIVE",
				EffectiveStartsAt: catBaseAt,
				EffectiveEndsAt:   catBaseAt.Add(90 * 24 * time.Hour),
				HasEffectiveEnd:   true,
				PublishedAt:       catBaseAt.Add(-24 * time.Hour),
				Form:              "NETWORK_SERVICE",
				HasForm:           true,
			},
			{
				ObjectID:          "product-2",
				VersionLabel:      "v1",
				Scope:             "scope-1",
				Status:            "PUBLISHED",
				EffectiveStartsAt: catBaseAt,
				PublishedAt:       catBaseAt.Add(-24 * time.Hour),
			},
		},
	}
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "SERVICE_PRODUCTS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	products, ok := body["products"].([]any)
	if !ok || len(products) != 2 {
		t.Fatalf("products = %v", body["products"])
	}

	first := products[0].(map[string]any)
	if first["objectId"] != "product-1" || first["form"] != "NETWORK_SERVICE" ||
		first["status"] != "EFFECTIVE" || first["scope"] != "scope-1" {
		t.Fatalf("首行变形:%v", first)
	}
	if first["effectiveStartsAt"] != "2026-08-25T08:00:00Z" {
		t.Fatalf("startsAt = %v", first["effectiveStartsAt"])
	}
	if _, has := first["effectiveEndsAt"]; !has {
		t.Fatal("有界区间的结束时刻没透出")
	}

	second := products[1].(map[string]any)
	if _, has := second["form"]; has {
		t.Fatal("未登形态的行长出了 form 键")
	}
	if _, has := second["effectiveEndsAt"]; has {
		t.Fatal("开放结束的行长出了 effectiveEndsAt 键")
	}
}

// Covers: ADR-0077 Decision 四——空目录走 2xx 成格、空数组不是 null,也不折成未配置。
func TestServiceProductsEndpointAnswersAnEmptyCatalogueWithAnEmptyArray(t *testing.T) {
	query := catalogueQuery(t)
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(
		intakeDouble{query: query},
		&productReaderDouble{tenant: query.Scope.Tenant()},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"products":[]`) {
		t.Fatalf("空目录不是空数组:%s", recorder.Body.String())
	}
}

func TestServiceProductsEndpointAnswersReadFailureWith500(t *testing.T) {
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&productReaderDouble{err: errors.New("库不可达")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q", code)
	}
}

// Covers: kind 封闭集是传输形状,先于 Intake——缺席或集外即 400,即便渠道未配置也
// 不折成 403;信用政策没有独立正文册,如实不在集合内。
func TestPoliciesEndpointRejectsMissingOrUnknownKindBeforeIntake(t *testing.T) {
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		commercialhttp.UnconfiguredIntake{},
		&policyReaderDouble{},
	)
	for _, target := range []string{
		"/commercial-policies",
		"/commercial-policies?kind=CREDIT_POLICY",
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", target, recorder.Code)
		}
		if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
			t.Fatalf("%s code = %q", target, code)
		}
	}
}

func TestPoliciesEndpointAnswersUnconfiguredIntakeWith403(t *testing.T) {
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		commercialhttp.UnconfiguredIntake{},
		&policyReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=PRICE_POLICY", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q", code)
	}
}

// Covers: 五种册子各自分派——每个 kind 只触发自己的读法,响应回显 kind;策略种类
// 间不串在传输面同样成立。
func TestPoliciesEndpointDispatchesEachKindToItsOwnList(t *testing.T) {
	query := catalogueQuery(t)
	tenant := query.Scope.Tenant()

	cases := []struct {
		kind   string
		method string
		seed   func(double *policyReaderDouble)
	}{
		{"ACCEPTANCE_RULE_PACKAGE", "packages", func(double *policyReaderDouble) {
			double.packages = []ports.AcceptanceRulePackageRow{{ObjectID: "rules-1", VersionLabel: "v1"}}
		}},
		{"PRE_ACCEPTANCE_CONTROL", "controls", func(double *policyReaderDouble) {
			double.controls = []ports.PreAcceptanceControlRow{{ContractObjectID: "contract-1", Requirement: "REQUIRED"}}
		}},
		{"PRICE_POLICY", "prices", func(double *policyReaderDouble) {
			double.prices = []ports.PricePolicyRow{{ObjectID: "price-1", Direction: "SELL"}}
		}},
		{"SETTLEMENT_POLICY", "settlements", func(double *policyReaderDouble) {
			double.settlements = []ports.SettlementPolicyRow{{ObjectID: "settle-1", Method: "TERMS"}}
		}},
		{"AS_OF_POLICY", "asOf", func(double *policyReaderDouble) {
			double.asOf = []ports.AsOfPolicyRow{{RulePackageObjectID: "rules-1", JudgmentType: "NETWORK_REACHABILITY"}}
		}},
	}
	for _, testCase := range cases {
		reader := &policyReaderDouble{tenant: tenant}
		testCase.seed(reader)
		endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/commercial-policies?kind="+testCase.kind, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", testCase.kind, recorder.Code, recorder.Body.String())
		}
		body := decodeBody(t, recorder)
		if body["outcome"] != "COMMERCIAL_POLICIES_LISTED" || body["kind"] != testCase.kind {
			t.Fatalf("%s outcome/kind = %v/%v", testCase.kind, body["outcome"], body["kind"])
		}
		policies, ok := body["policies"].([]any)
		if !ok || len(policies) != 1 {
			t.Fatalf("%s policies = %v", testCase.kind, body["policies"])
		}
		if len(reader.calls) != 1 || reader.calls[testCase.method] != 1 {
			t.Fatalf("%s 触发了别的读法:%v", testCase.kind, reader.calls)
		}
	}
}

// Covers: 规则包行体的嵌套转写——分类与引用逐条在场;开放结束时 effectiveEndsAt
// 键不在场;`不适用`声明的依据键只在带依据时在场。
func TestPoliciesEndpointTranscribesKindSpecificBodies(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		packages: []ports.AcceptanceRulePackageRow{{
			ObjectID:          "rules-1",
			VersionLabel:      "v1",
			ServiceProduct:    "product-1",
			Contract:          "contract-1",
			LegalEntity:       "legal-1",
			Scope:             "scope-1",
			EffectiveStartsAt: catBaseAt,
			DeclaredAt:        catBaseAt.Add(time.Hour),
			Rules: []ports.AssembledRuleRow{
				{Category: "MINIMUM_INGRESS_IDENTITY", Reference: "RULE/ingress-1"},
				{Category: "SHIPMENT_INVARIANT", Reference: "RULE/invariant-1"},
			},
		}},
		controls: []ports.PreAcceptanceControlRow{
			{ContractObjectID: "contract-1", ContractVersion: "v1", Requirement: "REQUIRED", DeclaredAt: catBaseAt},
			{ContractObjectID: "contract-2", ContractVersion: "v1", Requirement: "NOT_APPLICABLE",
				NotApplicableBasis: "CONTRACT-CLAUSE/NO-CONTROL", DeclaredAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE", nil))
	body := decodeBody(t, recorder)
	policies := body["policies"].([]any)
	pack := policies[0].(map[string]any)
	if pack["serviceProduct"] != "product-1" || pack["legalEntity"] != "legal-1" {
		t.Fatalf("五维变形:%v", pack)
	}
	if _, has := pack["effectiveEndsAt"]; has {
		t.Fatal("开放结束的规则包长出了 effectiveEndsAt 键")
	}
	rules, ok := pack["rules"].([]any)
	if !ok || len(rules) != 2 {
		t.Fatalf("rules = %v", pack["rules"])
	}
	firstRule := rules[0].(map[string]any)
	if firstRule["category"] != "MINIMUM_INGRESS_IDENTITY" || firstRule["reference"] != "RULE/ingress-1" {
		t.Fatalf("规则转写变形:%v", firstRule)
	}

	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=PRE_ACCEPTANCE_CONTROL", nil))
	body = decodeBody(t, recorder)
	controlRows := body["policies"].([]any)
	if len(controlRows) != 2 {
		t.Fatalf("controls = %v", body["policies"])
	}
	required := controlRows[0].(map[string]any)
	if _, has := required["notApplicableBasis"]; has {
		t.Fatal("要求控制的声明长出了不适用依据键")
	}
	waived := controlRows[1].(map[string]any)
	if waived["notApplicableBasis"] != "CONTRACT-CLAUSE/NO-CONTROL" {
		t.Fatalf("不适用依据没透出:%v", waived)
	}
}

func TestPoliciesEndpointAnswersReadFailureWith500(t *testing.T) {
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&policyReaderDouble{err: errors.New("库不可达")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=SETTLEMENT_POLICY", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q", code)
	}
}

// Covers: Intake 交回 ErrMalformedRequest 时按坏请求答——重发同样内容不会变好,
// 与渠道未配置(403)和翻译故障(5xx)分格。
func TestPoliciesEndpointAnswersMalformedIntakeWith400(t *testing.T) {
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		intakeDouble{err: commercialhttp.ErrMalformedRequest},
		&policyReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=AS_OF_POLICY", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q", code)
	}
}
