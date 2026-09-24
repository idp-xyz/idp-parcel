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
	authz       []ports.AuthorizationRuleRow
	credits     []ports.CreditPolicyRow
	serviceRule []ports.CustomerServiceRuleRow
	controlPols []ports.PreAcceptanceFinancialControlPolicyRow
	calls       map[string]int
	err         error
}

func (double *policyReaderDouble) ListPreAcceptanceFinancialControlPolicies(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.PreAcceptanceFinancialControlPolicyRow, error) {
	double.record("controlPolicies")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.controlPols, nil
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

func (double *policyReaderDouble) ListAuthorizationRules(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.AuthorizationRuleRow, error) {
	double.record("authz")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.authz, nil
}

func (double *policyReaderDouble) ListCreditPolicies(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.CreditPolicyRow, error) {
	double.record("credits")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.credits, nil
}

func (double *policyReaderDouble) ListCustomerServiceRules(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.CustomerServiceRuleRow, error) {
	double.record("serviceRules")
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.serviceRule, nil
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

// Covers: ADR-0055/ADR-0077 Decision 三——操作者渠道(ADR-0100)未就位,生产装配的 Intake 对
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
// 不折成 403;服务产品与客户合同是商业对象类别却不是**策略**册子(它们各有自己的目录端点),
// 如实不在本集合内。客户服务规则版本曾是这里的集外例子,随 0023 正文册落库进了集合。
func TestPoliciesEndpointRejectsMissingOrUnknownKindBeforeIntake(t *testing.T) {
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		commercialhttp.UnconfiguredIntake{},
		&policyReaderDouble{},
	)
	for _, target := range []string{
		"/commercial-policies",
		"/commercial-policies?kind=SERVICE_PRODUCT",
		"/commercial-policies?kind=CUSTOMER_CONTRACT",
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

// Covers: 未配置 Intake 对封闭集内全部 kind 同答——分派参数不能让调用方观察出
// 任何不同响应，且读口一次也不到达。
func TestPoliciesEndpointAnswersUnconfiguredIntakeIdenticallyForEveryKind(t *testing.T) {
	reader := &policyReaderDouble{}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(
		commercialhttp.UnconfiguredIntake{},
		reader,
	)

	var baseline string
	for _, kind := range []string{
		"ACCEPTANCE_RULE_PACKAGE",
		"PRE_ACCEPTANCE_CONTROL",
		"PRICE_POLICY",
		"SETTLEMENT_POLICY",
		"AS_OF_POLICY",
		"AUTHORIZATION_RULE",
		"CREDIT_POLICY",
		"CUSTOMER_SERVICE_RULE",
	} {
		recorder := httptest.NewRecorder()
		target := "/commercial-policies?kind=" + kind
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d", kind, recorder.Code)
		}
		if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s code = %q", kind, code)
		}
		if baseline == "" {
			baseline = recorder.Body.String()
		} else if recorder.Body.String() != baseline {
			t.Fatalf("%s 未配置答复与其他策略种类不一致:%s vs %s",
				kind, recorder.Body.String(), baseline)
		}
	}
	if len(reader.calls) != 0 {
		t.Fatalf("未配置 Intake 之后仍触发了读口:%v", reader.calls)
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
		{"CREDIT_POLICY", "credits", func(double *policyReaderDouble) {
			double.credits = []ports.CreditPolicyRow{{ObjectID: "credit-1", HasAmount: true, LimitMinor: 500000}}
		}},
		{"CUSTOMER_SERVICE_RULE", "serviceRules", func(double *policyReaderDouble) {
			double.serviceRule = []ports.CustomerServiceRuleRow{{ObjectID: "csr-1", VersionLabel: "v1"}}
		}},
		{"PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", "controlPolicies", func(double *policyReaderDouble) {
			double.controlPols = []ports.PreAcceptanceFinancialControlPolicyRow{{ObjectID: "fcp-1", VersionLabel: "v1"}}
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

// Covers: 信用政策行体的额度两格——金额行只长 limitMinor 键、比例行只长 limitRatioBasisPoints
// 键，另一格的键不在场。零额度是合法声明，所以在场与否不能靠零值判，靠的是行上的 HasAmount。
func TestPoliciesEndpointTranscribesCreditLimitAsExactlyOneKey(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		credits: []ports.CreditPolicyRow{
			{ObjectID: "credit-zero", VersionLabel: "v1", LegalEntity: "legal-1", AuthorityLevel: "level-commercial",
				ChargeType: "charge-freight", HasAmount: true, LimitMinor: 0,
				EffectiveStartsAt: catBaseAt, RegisteredAt: catBaseAt},
			{ObjectID: "credit-ratio", VersionLabel: "v1", LegalEntity: "legal-1", AuthorityLevel: "level-commercial",
				ChargeType: "charge-freight", LimitRatioBasisPoints: 1500,
				EffectiveStartsAt: catBaseAt, EffectiveEndsAt: catBaseAt.Add(time.Hour), HasEffectiveEnd: true,
				RegisteredAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=CREDIT_POLICY", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	rows := body["policies"].([]any)
	if len(rows) != 2 {
		t.Fatalf("policies = %v", body["policies"])
	}
	zero := rows[0].(map[string]any)
	if minor, has := zero["limitMinor"]; !has || minor != float64(0) {
		t.Fatalf("零金额额度没有以 limitMinor=0 在场:%v", zero)
	}
	if _, has := zero["limitRatioBasisPoints"]; has {
		t.Fatal("金额行长出了比例键")
	}
	if _, has := zero["effectiveEndsAt"]; has {
		t.Fatal("开放结束的信用政策长出了 effectiveEndsAt 键")
	}
	ratio := rows[1].(map[string]any)
	if bps, has := ratio["limitRatioBasisPoints"]; !has || bps != float64(1500) {
		t.Fatalf("比例额度没透出:%v", ratio)
	}
	if _, has := ratio["limitMinor"]; has {
		t.Fatal("比例行长出了金额键")
	}
	if ratio["effectiveEndsAt"] == nil || ratio["chargeType"] != "charge-freight" {
		t.Fatalf("其余字段没有照列:%v", ratio)
	}
}

// Covers: 客户服务规则行体的转写（票 party-commercial-context-gaps/05，ADR-0104）——contentRegistered 说明
// 这一版登没登正文：只有壳的版本 contentRegistered 为假且 content 键不在场（那正是 VE 点读答未登记的
// 状态，目录不得让它消失）；登了正文的行 content 节在场，适用对象恰一键（serviceProduct / customerContract）、
// 期限与材料两数组各自成形，无客户差异的那一项是空数组不是缺键。
func TestPoliciesEndpointTranscribesCustomerServiceRuleContentOnlyWhenRegistered(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		serviceRule: []ports.CustomerServiceRuleRow{
			{ObjectID: "csr-bare", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			{ObjectID: "csr-full", VersionLabel: "v2", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, EffectiveEndsAt: catBaseAt.Add(time.Hour), HasEffectiveEnd: true,
				PublishedAt: catBaseAt,
				HasContent:  true, CustomerContract: "contract-1", ResponsibleParty: "operator-1", RuleScope: "scope-1",
				RegisteredAt: catBaseAt.Add(time.Minute),
				ClaimDeadlines: []ports.ClaimDeadlineRow{
					{Kind: "FIRST_CLAIM", StartEvent: "event-delivered", DurationDays: 30, Calendar: "calendar-cn"},
				},
				MinimumMaterials: []ports.MinimumMaterialsRow{
					{ClaimKind: "claim-loss", Materials: []string{"material-invoice", "material-photo"}},
				}},
			{ObjectID: "csr-product", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
				HasContent: true, ServiceProduct: "product-1", ResponsibleParty: "operator-1", RuleScope: "scope-1",
				RegisteredAt:     catBaseAt,
				MinimumMaterials: []ports.MinimumMaterialsRow{{ClaimKind: "claim-damage", Materials: []string{"material-photo"}}}},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=CUSTOMER_SERVICE_RULE", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["kind"] != "CUSTOMER_SERVICE_RULE" {
		t.Fatalf("kind = %v", body["kind"])
	}
	rows := body["policies"].([]any)
	if len(rows) != 3 {
		t.Fatalf("policies = %v", body["policies"])
	}

	bare := rows[0].(map[string]any)
	if bare["contentRegistered"] != false {
		t.Fatalf("只有壳的版本 contentRegistered = %v", bare["contentRegistered"])
	}
	if _, has := bare["content"]; has {
		t.Fatal("没登正文的版本长出了 content 节")
	}
	if _, has := bare["effectiveEndsAt"]; has {
		t.Fatal("开放结束的版本长出了 effectiveEndsAt 键")
	}
	if bare["objectId"] != "csr-bare" || bare["status"] != "EFFECTIVE" {
		t.Fatalf("壳自身的字段没有照列:%v", bare)
	}

	full := rows[1].(map[string]any)
	if full["contentRegistered"] != true || full["effectiveEndsAt"] == nil {
		t.Fatalf("登了正文的版本 = %v", full)
	}
	content, ok := full["content"].(map[string]any)
	if !ok {
		t.Fatalf("content 节缺席:%v", full)
	}
	if content["customerContract"] != "contract-1" || content["responsibleParty"] != "operator-1" ||
		content["scope"] != "scope-1" || content["registeredAt"] == nil {
		t.Fatalf("正文字段变形:%v", content)
	}
	if _, has := content["serviceProduct"]; has {
		t.Fatal("按合同适用的正文长出了 serviceProduct 键")
	}
	deadlines, ok := content["claimDeadlines"].([]any)
	if !ok || len(deadlines) != 1 {
		t.Fatalf("claimDeadlines = %v", content["claimDeadlines"])
	}
	deadline := deadlines[0].(map[string]any)
	if deadline["kind"] != "FIRST_CLAIM" || deadline["startEvent"] != "event-delivered" ||
		deadline["durationDays"] != float64(30) || deadline["calendar"] != "calendar-cn" {
		t.Fatalf("期限转写变形:%v", deadline)
	}
	materials, ok := content["minimumMaterials"].([]any)
	if !ok || len(materials) != 1 {
		t.Fatalf("minimumMaterials = %v", content["minimumMaterials"])
	}
	entry := materials[0].(map[string]any)
	if entry["claimKind"] != "claim-loss" {
		t.Fatalf("材料转写变形:%v", entry)
	}
	if list, ok := entry["materials"].([]any); !ok || len(list) != 2 || list[0] != "material-invoice" {
		t.Fatalf("材料清单变形:%v", entry["materials"])
	}

	product := rows[2].(map[string]any)["content"].(map[string]any)
	if product["serviceProduct"] != "product-1" {
		t.Fatalf("按产品适用的正文没透出产品:%v", product)
	}
	if _, has := product["customerContract"]; has {
		t.Fatal("按产品适用的正文长出了 customerContract 键")
	}
	if list, ok := product["claimDeadlines"].([]any); !ok || len(list) != 0 {
		t.Fatalf("无期限差异的正文该是空数组而不是缺键:%v", product["claimDeadlines"])
	}
}

// Covers: 价格政策行体的口径转写（票 party-commercial-context-gaps/06）——caliberDeclared 说明有没有
// 口径（0010 早于 0022，只有正文没有口径的行合法），有则 caliber 节在场：分类只在含税/未税时在场、
// 系数只在 SELL 时在场、fx 节只在声明了汇率口径时在场——三处缺席都是口径说出的真话，不补空串。
func TestPoliciesEndpointTranscribesPriceCaliberOnlyWhenDeclared(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		prices: []ports.PricePolicyRow{
			{ObjectID: "price-fx", VersionLabel: "v1", Direction: "SELL", PlanRef: "plan-buy-1", PlanDirection: "BUY",
				BindingConversion: "FROZEN_BUY_EVALUATION", PolicyScope: "scope-1",
				EffectiveStartsAt: catBaseAt, RegisteredAt: catBaseAt,
				HasCaliber: true, TaxDisposition: "TAX_INCLUSIVE", TaxClassification: "vat-standard",
				VolumetricFactor: "sell-divisor-5000-cm",
				HasFx:            true, FxQuoteType: "boc-cash-selling", FxAsOfSemantics: "AT_ORDER_DATE", FxAsOfPolicyVersion: "asof-policy/v3",
				CaliberRegisteredAt: catBaseAt.Add(time.Minute)},
			{ObjectID: "price-plain", VersionLabel: "v1", Direction: "BUY", PlanRef: "plan-buy-2", PlanDirection: "BUY",
				BindingConversion: "NONE", PolicyScope: "scope-1",
				EffectiveStartsAt: catBaseAt, RegisteredAt: catBaseAt,
				HasCaliber: true, TaxDisposition: "TAX_NOT_APPLICABLE", CaliberRegisteredAt: catBaseAt},
			{ObjectID: "price-bare", VersionLabel: "v1", Direction: "SELL", PlanRef: "plan-sell-1", PlanDirection: "SELL",
				BindingConversion: "NONE", PolicyScope: "scope-1",
				EffectiveStartsAt: catBaseAt, RegisteredAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=PRICE_POLICY", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	rows := body["policies"].([]any)
	if len(rows) != 3 {
		t.Fatalf("policies = %v", body["policies"])
	}

	withFx := rows[0].(map[string]any)
	if withFx["caliberDeclared"] != true || withFx["planDirection"] != "BUY" {
		t.Fatalf("含口径的行 = %v", withFx)
	}
	caliber, ok := withFx["caliber"].(map[string]any)
	if !ok {
		t.Fatalf("caliber 节没透出:%v", withFx)
	}
	if caliber["taxDisposition"] != "TAX_INCLUSIVE" || caliber["taxClassification"] != "vat-standard" ||
		caliber["volumetricFactor"] != "sell-divisor-5000-cm" || caliber["registeredAt"] == nil {
		t.Fatalf("口径转写变形:%v", caliber)
	}
	fx, ok := caliber["fx"].(map[string]any)
	if !ok || fx["quoteType"] != "boc-cash-selling" || fx["asOfSemantics"] != "AT_ORDER_DATE" || fx["asOfPolicyVersion"] != "asof-policy/v3" {
		t.Fatalf("汇率口径转写变形:%v", caliber["fx"])
	}

	plain := rows[1].(map[string]any)
	caliber = plain["caliber"].(map[string]any)
	if caliber["taxDisposition"] != "TAX_NOT_APPLICABLE" {
		t.Fatalf("不适用的税务口径没透出:%v", caliber)
	}
	for _, key := range []string{"taxClassification", "volumetricFactor", "fx"} {
		if _, has := caliber[key]; has {
			t.Fatalf("缺席的 %s 长出了键:%v", key, caliber)
		}
	}

	bare := rows[2].(map[string]any)
	if bare["caliberDeclared"] != false {
		t.Fatalf("无口径的行 caliberDeclared = %v", bare["caliberDeclared"])
	}
	if _, has := bare["caliber"]; has {
		t.Fatal("没登记口径却长出了 caliber 节")
	}
	if bare["direction"] != "SELL" || bare["planRef"] != "plan-sell-1" {
		t.Fatalf("正文字段没有照列:%v", bare)
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
