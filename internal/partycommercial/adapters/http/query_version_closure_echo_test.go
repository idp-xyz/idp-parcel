package commercialhttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// Covers: `AT-PC-065` 在 HTTP 答复体上的那一半——服务产品、客户合同、授权规则、接受前财务控制策略、客户服务规则五本目录
// 与供应商协议同键回显收尾（票 demo-chain-gaps/184）：截止的行带 selectionEndsAt，退役的行带 successorVersion 与 retiredAt，
// 没收尾的行三键一个不长；没登正文的壳行同样回显。
func TestFiveCatalogueEndpointsEchoTheVersionClosure(t *testing.T) {
	query := catalogueQuery(t)
	tenant := query.Scope.Tenant()
	cutAt, retiredAt := catBaseAt.Add(24*time.Hour), catBaseAt.Add(48*time.Hour)
	intake := intakeDouble{query: query}

	cases := []struct {
		name    string
		path    string
		listKey string
		handler http.Handler
	}{
		{"服务产品", "/commercial-service-products", "products", commercialhttp.NewQueryServiceProductsEndpoint(intake,
			&productReaderDouble{tenant: tenant, rows: []ports.ServiceProductCatalogueRow{
				{ObjectID: "o", VersionLabel: "v1", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					SelectionEndsAt: cutAt, HasSelectionCut: true},
				{ObjectID: "o", VersionLabel: "v2", Scope: "s", Status: "RETIRED", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					Successor: "v3", RetiredAt: retiredAt},
				{ObjectID: "o", VersionLabel: "v3", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			}})},
		{"客户合同", "/commercial-customer-contracts", "contracts", commercialhttp.NewQueryCustomerContractsEndpoint(intake,
			&relationReaderDouble{tenant: tenant, contracts: []ports.CustomerContractCatalogueRow{
				{ObjectID: "o", VersionLabel: "v1", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					SelectionEndsAt: cutAt, HasSelectionCut: true},
				{ObjectID: "o", VersionLabel: "v2", Scope: "s", Status: "RETIRED", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					Successor: "v3", RetiredAt: retiredAt},
				{ObjectID: "o", VersionLabel: "v3", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			}})},
		{"授权规则", "/commercial-policies?kind=AUTHORIZATION_RULE", "policies", commercialhttp.NewQueryCommercialPoliciesEndpoint(intake,
			&policyReaderDouble{tenant: tenant, authz: []ports.AuthorizationRuleRow{
				{ObjectID: "o", VersionLabel: "v1", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					SelectionEndsAt: cutAt, HasSelectionCut: true},
				{ObjectID: "o", VersionLabel: "v2", Scope: "s", Status: "RETIRED", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					Successor: "v3", RetiredAt: retiredAt},
				{ObjectID: "o", VersionLabel: "v3", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			}})},
		{"接受前财务控制策略", "/commercial-policies?kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", "policies", commercialhttp.NewQueryCommercialPoliciesEndpoint(intake,
			&policyReaderDouble{tenant: tenant, controlPols: []ports.PreAcceptanceFinancialControlPolicyRow{
				{ObjectID: "o", VersionLabel: "v1", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					SelectionEndsAt: cutAt, HasSelectionCut: true},
				{ObjectID: "o", VersionLabel: "v2", Scope: "s", Status: "RETIRED", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					Successor: "v3", RetiredAt: retiredAt},
				{ObjectID: "o", VersionLabel: "v3", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			}})},
		{"客户服务规则", "/commercial-policies?kind=CUSTOMER_SERVICE_RULE", "policies", commercialhttp.NewQueryCommercialPoliciesEndpoint(intake,
			&policyReaderDouble{tenant: tenant, serviceRule: []ports.CustomerServiceRuleRow{
				{ObjectID: "o", VersionLabel: "v1", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					SelectionEndsAt: cutAt, HasSelectionCut: true},
				{ObjectID: "o", VersionLabel: "v2", Scope: "s", Status: "RETIRED", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
					Successor: "v3", RetiredAt: retiredAt},
				{ObjectID: "o", VersionLabel: "v3", Scope: "s", Status: "EFFECTIVE", EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			tc.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			rows, _ := decodeBody(t, recorder)[tc.listKey].([]any)
			if len(rows) != 3 {
				t.Fatalf("%s = %v", tc.listKey, rows)
			}
			cut, retired, plain := rows[0].(map[string]any), rows[1].(map[string]any), rows[2].(map[string]any)
			if cut["selectionEndsAt"] == nil || cut["successorVersion"] != nil || cut["retiredAt"] != nil {
				t.Fatalf("截止的行 = %v", cut)
			}
			if retired["status"] != "RETIRED" || retired["successorVersion"] != "v3" || retired["retiredAt"] == nil || retired["selectionEndsAt"] != nil {
				t.Fatalf("退役的行 = %v", retired)
			}
			for _, key := range []string{"selectionEndsAt", "successorVersion", "retiredAt"} {
				if _, has := plain[key]; has {
					t.Fatalf("没收尾的行长出了 %q：%v", key, plain)
				}
			}
		})
	}
}
