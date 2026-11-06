package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对商业版本按身份读回端点（ADR-0361 决定一）证传输面：在册答 READ 并带与该类别目录行同形的一份；不在册与跨租户
// 同答 200 加 NOT_FOUND；类别按领域原词判、目录页签名不算；Intake 先于路径；方法门、读失败各自的答复。

// versionReaderDouble 同时扮三个目录读口。按身份读回只认（租户、对象、版本）三格全同的那一行，跨租户与键不同同形（读不到），
// 与真库一致；每次 Find 记下被叫的是哪一册，证类别分派指对了读口。
type versionReaderDouble struct {
	tenant    domain.TenantID
	product   *ports.ServiceProductCatalogueRow
	agreement *ports.SupplierAgreementCatalogueRow
	calls     []string
	err       error
}

func (double *versionReaderDouble) hit(method string, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel, rowObject, rowVersion string) (bool, error) {
	double.calls = append(double.calls, method)
	if double.err != nil {
		return false, double.err
	}
	return tenant == double.tenant && objectID.String() == rowObject && version.String() == rowVersion, nil
}

func (double *versionReaderDouble) FindServiceProduct(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.ServiceProductCatalogueRow, bool, error) {
	if double.product == nil {
		found, err := double.hit("FindServiceProduct", tenant, objectID, version, "", "")
		return ports.ServiceProductCatalogueRow{}, found, err
	}
	found, err := double.hit("FindServiceProduct", tenant, objectID, version, double.product.ObjectID, double.product.VersionLabel)
	if !found {
		return ports.ServiceProductCatalogueRow{}, false, err
	}
	return *double.product, true, nil
}

func (double *versionReaderDouble) FindSupplierAgreement(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.SupplierAgreementCatalogueRow, bool, error) {
	if double.agreement == nil {
		found, err := double.hit("FindSupplierAgreement", tenant, objectID, version, "", "")
		return ports.SupplierAgreementCatalogueRow{}, found, err
	}
	found, err := double.hit("FindSupplierAgreement", tenant, objectID, version, double.agreement.ObjectID, double.agreement.VersionLabel)
	if !found {
		return ports.SupplierAgreementCatalogueRow{}, false, err
	}
	return *double.agreement, true, nil
}

func (double *versionReaderDouble) FindCustomerContract(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.CustomerContractCatalogueRow, bool, error) {
	found, err := double.hit("FindCustomerContract", tenant, objectID, version, "", "")
	return ports.CustomerContractCatalogueRow{}, found, err
}

func (double *versionReaderDouble) FindAcceptanceRulePackage(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.AcceptanceRulePackageRow, bool, error) {
	found, err := double.hit("FindAcceptanceRulePackage", tenant, objectID, version, "", "")
	return ports.AcceptanceRulePackageRow{}, found, err
}

func (double *versionReaderDouble) FindPricePolicy(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.PricePolicyRow, bool, error) {
	found, err := double.hit("FindPricePolicy", tenant, objectID, version, "", "")
	return ports.PricePolicyRow{}, found, err
}

func (double *versionReaderDouble) FindSettlementPolicy(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.SettlementPolicyRow, bool, error) {
	found, err := double.hit("FindSettlementPolicy", tenant, objectID, version, "", "")
	return ports.SettlementPolicyRow{}, found, err
}

func (double *versionReaderDouble) FindAuthorizationRule(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.AuthorizationRuleRow, bool, error) {
	found, err := double.hit("FindAuthorizationRule", tenant, objectID, version, "", "")
	return ports.AuthorizationRuleRow{}, found, err
}

func (double *versionReaderDouble) FindCreditPolicy(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.CreditPolicyRow, bool, error) {
	found, err := double.hit("FindCreditPolicy", tenant, objectID, version, "", "")
	return ports.CreditPolicyRow{}, found, err
}

func (double *versionReaderDouble) FindCustomerServiceRule(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.CustomerServiceRuleRow, bool, error) {
	found, err := double.hit("FindCustomerServiceRule", tenant, objectID, version, "", "")
	return ports.CustomerServiceRuleRow{}, found, err
}

func (double *versionReaderDouble) FindPreAcceptanceFinancialControlPolicy(
	_ context.Context, tenant domain.TenantID, objectID domain.CommercialObjectID, version domain.CommercialVersionLabel,
) (ports.PreAcceptanceFinancialControlPolicyRow, bool, error) {
	found, err := double.hit("FindPreAcceptanceFinancialControlPolicy", tenant, objectID, version, "", "")
	return ports.PreAcceptanceFinancialControlPolicyRow{}, found, err
}

// 列表那一面本端点不碰；替身答空只为满足读口形状。
func (double *versionReaderDouble) ListServiceProducts(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.ServiceProductCatalogueRow], error) {
	return cataloguepage.Result[ports.ServiceProductCatalogueRow]{}, nil
}
func (double *versionReaderDouble) ListCustomerContracts(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerContractCatalogueRow], error) {
	return cataloguepage.Result[ports.CustomerContractCatalogueRow]{}, nil
}
func (double *versionReaderDouble) ListSupplierAgreements(
	context.Context, domain.TenantID, int, cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierAgreementCatalogueRow], error) {
	return cataloguepage.Result[ports.SupplierAgreementCatalogueRow]{}, nil
}
func (double *versionReaderDouble) ListAcceptanceRulePackages(context.Context, domain.TenantID, int) ([]ports.AcceptanceRulePackageRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListPreAcceptanceControls(context.Context, domain.TenantID, int) ([]ports.PreAcceptanceControlRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListPricePolicies(context.Context, domain.TenantID, int) ([]ports.PricePolicyRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListSettlementPolicies(context.Context, domain.TenantID, int) ([]ports.SettlementPolicyRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListAsOfPolicyDeclarations(context.Context, domain.TenantID, int) ([]ports.AsOfPolicyRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListAuthorizationRules(context.Context, domain.TenantID, int) ([]ports.AuthorizationRuleRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListCreditPolicies(context.Context, domain.TenantID, int) ([]ports.CreditPolicyRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListCustomerServiceRules(context.Context, domain.TenantID, int) ([]ports.CustomerServiceRuleRow, error) {
	return nil, nil
}
func (double *versionReaderDouble) ListPreAcceptanceFinancialControlPolicies(context.Context, domain.TenantID, int) ([]ports.PreAcceptanceFinancialControlPolicyRow, error) {
	return nil, nil
}

func versionReadbackEndpoint(intake commercialhttp.CommercialCatalogueIntake, reader *versionReaderDouble) http.Handler {
	return commercialhttp.NewQueryCommercialVersionEndpoint(intake, reader, reader, reader)
}

func versionReadbackRequest(kind, objectID, version string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/commercial-version-readbacks/"+kind+"/"+objectID+"/"+version, nil)
	// 单测不经路由，路径参数由这里按路由会做的方式填进去；装配后的形状由 cmd/parcel-api 的隔离读用例经真路由再证一次。
	request.SetPathValue("kind", kind)
	request.SetPathValue("objectId", objectID)
	request.SetPathValue("version", version)
	return request
}

func decodeVersionReadback(t *testing.T, recorder *httptest.ResponseRecorder) (string, string, map[string]json.RawMessage) {
	t.Helper()
	var body struct {
		Outcome string                     `json:"outcome"`
		Kind    string                     `json:"kind"`
		Version map[string]json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}
	return body.Outcome, body.Kind, body.Version
}

// Covers: ADR-0361 决定一——在册答 COMMERCIAL_VERSION_READ，kind 回显领域原词，version 与服务产品目录列表上的那一行逐键同形。
func TestACommercialVersionIsReadBackAsTheRowItsCatalogueLists(t *testing.T) {
	query := catalogueQuery(t)
	row := ports.ServiceProductCatalogueRow{
		ObjectID: "product-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pcNow, PublishedAt: pcNow, Form: "NETWORK_SERVICE", HasForm: true,
	}
	reader := &versionReaderDouble{tenant: query.Scope.Tenant(), product: &row}
	recorder := httptest.NewRecorder()
	versionReadbackEndpoint(intakeDouble{query: query}, reader).ServeHTTP(recorder, versionReadbackRequest("SERVICE_PRODUCT", "product-1", "v1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	outcome, kind, version := decodeVersionReadback(t, recorder)
	if outcome != "COMMERCIAL_VERSION_READ" || kind != "SERVICE_PRODUCT" {
		t.Fatalf("outcome/kind = %q/%q", outcome, kind)
	}
	if string(version["objectId"]) != `"product-1"` || string(version["form"]) != `"NETWORK_SERVICE"` {
		t.Fatalf("version = %s", recorder.Body)
	}

	// 同一行经目录列表端点折出的那一份：按身份读回是列表的投影，两边必须逐键相同，对象页显示的才是目录上这一版。
	listed := httptest.NewRecorder()
	commercialhttp.NewQueryServiceProductsEndpoint(intakeDouble{query: query}, &productReaderDouble{tenant: query.Scope.Tenant(), rows: []ports.ServiceProductCatalogueRow{row}}).
		ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))
	var list struct {
		Products []map[string]json.RawMessage `json:"products"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Products) != 1 {
		t.Fatalf("列表 = %s (%v)", listed.Body, err)
	}
	if !reflect.DeepEqual(stringKeys(list.Products[0]), stringKeys(version)) {
		t.Fatalf("按身份读回与目录行不同形：\n读回 %s\n列表 %s", recorder.Body, listed.Body)
	}
}

func stringKeys(raw map[string]json.RawMessage) map[string]string {
	keys := make(map[string]string, len(raw))
	for key, value := range raw {
		keys[key] = string(value)
	}
	return keys
}

// Covers: ADR-0361 决定一——不在册、键差一字、别的租户三者同答 200 加 COMMERCIAL_VERSION_NOT_FOUND，且答复不带 kind 与 version
// （状态码只说答案有没有形成，ADR-0022；不泄露存在性）。
func TestAnAbsentOrForeignCommercialVersionIsAnsweredAsNotFound(t *testing.T) {
	query := catalogueQuery(t)
	row := ports.SupplierAgreementCatalogueRow{ObjectID: "agreement-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE"}
	for name, reader := range map[string]*versionReaderDouble{
		"键差一字": {tenant: query.Scope.Tenant(), agreement: &ports.SupplierAgreementCatalogueRow{ObjectID: "agreement-1", VersionLabel: "v2"}},
		"别的租户": {tenant: catValue(t, domain.NewTenantID, "tenant-2"), agreement: &row},
		"不在册":  {tenant: query.Scope.Tenant()},
	} {
		recorder := httptest.NewRecorder()
		versionReadbackEndpoint(intakeDouble{query: query}, reader).ServeHTTP(recorder, versionReadbackRequest("SUPPLIER_AGREEMENT", "agreement-1", "v1"))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", name, recorder.Code, recorder.Body)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if string(body["outcome"]) != `"COMMERCIAL_VERSION_NOT_FOUND"` || len(body) != 1 {
			t.Fatalf("%s: body = %s", name, recorder.Body)
		}
	}

	// 在册的那一份照常读得回，证上面三格不是替身恒答不在。
	recorder := httptest.NewRecorder()
	versionReadbackEndpoint(intakeDouble{query: query}, &versionReaderDouble{tenant: query.Scope.Tenant(), agreement: &row}).
		ServeHTTP(recorder, versionReadbackRequest("SUPPLIER_AGREEMENT", "agreement-1", "v1"))
	if outcome, kind, version := decodeVersionReadback(t, recorder); outcome != "COMMERCIAL_VERSION_READ" || kind != "SUPPLIER_AGREEMENT" || string(version["objectId"]) != `"agreement-1"` {
		t.Fatalf("在册的协议 = %s", recorder.Body)
	}
}

// Covers: ADR-0361 决定一——类别是领域封闭集的原词，每个类别分派到自己那一册的读口；目录页签名（PRICE_POLICY）不是类别。
func TestEachCommercialObjectKindIsDispatchedToItsOwnCatalogueRead(t *testing.T) {
	query := catalogueQuery(t)
	for kind, method := range map[string]string{
		"SERVICE_PRODUCT":                         "FindServiceProduct",
		"CUSTOMER_CONTRACT":                       "FindCustomerContract",
		"SUPPLIER_AGREEMENT":                      "FindSupplierAgreement",
		"ACCEPTANCE_RULE_PACKAGE":                 "FindAcceptanceRulePackage",
		"PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY": "FindPreAcceptanceFinancialControlPolicy",
		"PRICE_RULE":                              "FindPricePolicy",
		"SETTLEMENT_POLICY":                       "FindSettlementPolicy",
		"CREDIT_POLICY":                           "FindCreditPolicy",
		"AUTHORIZATION_RULE":                      "FindAuthorizationRule",
		"CUSTOMER_SERVICE_RULE":                   "FindCustomerServiceRule",
	} {
		reader := &versionReaderDouble{tenant: query.Scope.Tenant()}
		recorder := httptest.NewRecorder()
		versionReadbackEndpoint(intakeDouble{query: query}, reader).ServeHTTP(recorder, versionReadbackRequest(kind, "object-1", "v1"))
		if recorder.Code != http.StatusOK || !reflect.DeepEqual(reader.calls, []string{method}) {
			t.Fatalf("%s: status = %d calls = %v, want 200 via %s", kind, recorder.Code, reader.calls, method)
		}
	}

	for _, malformed := range [][3]string{
		{"PRICE_POLICY", "object-1", "v1"}, // 目录列表页签的名字，不是商业对象类别
		{"AS_OF_POLICY", "object-1", "v1"}, // 声明没有自己的版本
		{"SERVICE_PRODUCT", "", "v1"},
		{"SERVICE_PRODUCT", "object-1", ""},
	} {
		reader := &versionReaderDouble{tenant: query.Scope.Tenant()}
		recorder := httptest.NewRecorder()
		versionReadbackEndpoint(intakeDouble{query: query}, reader).ServeHTTP(recorder, versionReadbackRequest(malformed[0], malformed[1], malformed[2]))
		if recorder.Code != http.StatusBadRequest || len(reader.calls) != 0 {
			t.Fatalf("%v: status = %d calls = %v, want 400 且不触读口", malformed, recorder.Code, reader.calls)
		}
	}
}

// Covers: ADR-0055——Intake 先于路径：渠道未配置时类别再畸形也答 403，路径内容一个字不读；方法门答 405 带 Allow；读失败答 5xx 不伪装成不在册。
func TestCommercialVersionReadbackIntakesBeforeThePathAndAnswersFailuresAsNoAnswer(t *testing.T) {
	query := catalogueQuery(t)
	reader := &versionReaderDouble{tenant: query.Scope.Tenant()}
	unconfigured := httptest.NewRecorder()
	versionReadbackEndpoint(intakeDouble{err: commercialhttp.ErrAccessChannelNotConfigured}, reader).
		ServeHTTP(unconfigured, versionReadbackRequest("NOT_A_KIND", "", ""))
	if unconfigured.Code != http.StatusForbidden || len(reader.calls) != 0 {
		t.Fatalf("未配置 = %d calls = %v, want 403 且不触读口", unconfigured.Code, reader.calls)
	}

	post := httptest.NewRecorder()
	versionReadbackEndpoint(intakeDouble{query: query}, reader).ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d allow=%q", post.Code, post.Header().Get("Allow"))
	}

	failed := httptest.NewRecorder()
	versionReadbackEndpoint(intakeDouble{query: query}, &versionReaderDouble{tenant: query.Scope.Tenant(), err: errors.New("boom")}).
		ServeHTTP(failed, versionReadbackRequest("CREDIT_POLICY", "credit-1", "v1"))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 = %d %s, want 500", failed.Code, failed.Body)
	}
}
