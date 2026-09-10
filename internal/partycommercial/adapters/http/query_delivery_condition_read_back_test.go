package commercialhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证两张目录端点上交付条件那一节的转写（票 admin-write-faces/25 裁读回折进既有目录行）：声明过的行带
// deliveryConditions 一节且键名镜像批文（合同层多 tightens），没声明的行整键不在场——页面先看键在不在，不拿空数组兼作「没有」。

type contractReaderDouble struct {
	tenant    domain.TenantID
	contracts []ports.CustomerContractCatalogueRow
}

func (double *contractReaderDouble) ListCustomerContracts(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.CustomerContractCatalogueRow, error) {
	if tenant != double.tenant {
		return nil, nil
	}
	return double.contracts, nil
}

func (double *contractReaderDouble) ListSupplierAgreements(
	context.Context, domain.TenantID, int,
) ([]ports.SupplierAgreementCatalogueRow, error) {
	return nil, nil
}

func declaredConditionsRow(tightensObject, tightensVersion string) *ports.DeliveryConditionCatalogueRow {
	return &ports.DeliveryConditionCatalogueRow{
		Methods:             []string{"METHOD/in-person", "METHOD/locker"},
		RecipientScopeRule:  "RULE/recipient-scope-1",
		ProofOfDeliveryRule: "RULE/proof-1",
		TightensObjectID:    tightensObject,
		TightensVersion:     tightensVersion,
		DeclaredAt:          catBaseAt,
	}
}

// Covers: 服务产品行——产品层一节逐键透出、无 tightens 键；没声明的行没有 deliveryConditions 键。
func TestServiceProductsEndpointTranscribesDeliveryConditionsOnlyWhenDeclared(t *testing.T) {
	query := catalogueQuery(t)
	reader := &productReaderDouble{
		tenant: query.Scope.Tenant(),
		rows: []ports.ServiceProductCatalogueRow{
			{ObjectID: "product-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt, DeliveryConditions: declaredConditionsRow("", "")},
			{ObjectID: "product-2", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryServiceProductsEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-service-products", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	rows := decodeBody(t, recorder)["products"].([]any)
	declared := rows[0].(map[string]any)
	conditions, ok := declared["deliveryConditions"].(map[string]any)
	if !ok {
		t.Fatalf("声明过的行没有 deliveryConditions 一节：%v", declared)
	}
	methods, _ := conditions["methods"].([]any)
	if len(methods) != 2 || methods[0] != "METHOD/in-person" || conditions["recipientScopeRule"] != "RULE/recipient-scope-1" ||
		conditions["proofOfDeliveryRule"] != "RULE/proof-1" || conditions["declaredAt"] == nil {
		t.Fatalf("一节没有逐键透出：%v", conditions)
	}
	if _, has := conditions["tightens"]; has {
		t.Fatalf("产品层长出了 tightens 键：%v", conditions)
	}
	if _, has := rows[1].(map[string]any)["deliveryConditions"]; has {
		t.Fatalf("没声明的行长出了 deliveryConditions 键：%v", rows[1])
	}
}

// Covers: 客户合同行——合同层一节带 tightens 两键；交付条件与正文两层各自可缺：只登交付条件的行 contentRegistered 为假而
// 一节在场，只登正文的行反之。
func TestCustomerContractsEndpointTranscribesDeliveryConditionsAsAThirdLayer(t *testing.T) {
	query := catalogueQuery(t)
	reader := &contractReaderDouble{
		tenant: query.Scope.Tenant(),
		contracts: []ports.CustomerContractCatalogueRow{
			{ObjectID: "contract-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
				DeliveryConditions: declaredConditionsRow("product-1", "v1")},
			{ObjectID: "contract-2", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
				HasContent: true, RulePackageID: "rules-1", DeclaredAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryCustomerContractsEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-customer-contracts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	rows := decodeBody(t, recorder)["contracts"].([]any)
	conditionsOnly := rows[0].(map[string]any)
	if conditionsOnly["contentRegistered"] != false {
		t.Fatalf("只登交付条件的行 contentRegistered = %v", conditionsOnly["contentRegistered"])
	}
	conditions, ok := conditionsOnly["deliveryConditions"].(map[string]any)
	if !ok {
		t.Fatalf("合同层没有透出：%v", conditionsOnly)
	}
	tightens, _ := conditions["tightens"].(map[string]any)
	if tightens["objectId"] != "product-1" || tightens["version"] != "v1" {
		t.Fatalf("合同层没带所收紧的产品版本：%v", conditions)
	}
	contentOnly := rows[1].(map[string]any)
	if contentOnly["contentRegistered"] != true {
		t.Fatalf("只登正文的行 contentRegistered = %v", contentOnly["contentRegistered"])
	}
	if _, has := contentOnly["deliveryConditions"]; has {
		t.Fatalf("只登正文的行长出了 deliveryConditions 键：%v", contentOnly)
	}
}
