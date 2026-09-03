package commercialhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对供应商协议目录端点证正文透出（票 party-commercial-context-gaps/03）：壳在正文缺时
// contentRegistered 为假且正文键一个不长；正文在场时逐字段透出。

type relationReaderDouble struct {
	tenant     domain.TenantID
	agreements []ports.SupplierAgreementCatalogueRow
}

func (double *relationReaderDouble) ListCustomerContracts(
	context.Context, domain.TenantID, int,
) ([]ports.CustomerContractCatalogueRow, error) {
	return nil, nil
}

func (double *relationReaderDouble) ListSupplierAgreements(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.SupplierAgreementCatalogueRow, error) {
	if tenant != double.tenant {
		return nil, nil
	}
	return double.agreements, nil
}

func TestSupplierAgreementsEndpointTranscribesContentOnlyWhenRegistered(t *testing.T) {
	query := catalogueQuery(t)
	reader := &relationReaderDouble{
		tenant: query.Scope.Tenant(),
		agreements: []ports.SupplierAgreementCatalogueRow{
			{ObjectID: "agreement-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
				HasContent: true, Supplier: "supplier-1", LegalEntity: "legal-1", PurchasePlan: "plan-buy-1",
				AgreementScope: "scope-procurement", AgreementEffectiveStartsAt: catBaseAt,
				AgreementEffectiveEndsAt: catBaseAt.Add(24 * time.Hour), HasAgreementEffectiveEnd: true,
				RegisteredAt: catBaseAt.Add(time.Hour)},
			{ObjectID: "agreement-2", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQuerySupplierAgreementsEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-supplier-agreements", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "SUPPLIER_AGREEMENTS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	rows, ok := body["agreements"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("agreements = %v", body["agreements"])
	}

	full := rows[0].(map[string]any)
	if full["contentRegistered"] != true || full["supplier"] != "supplier-1" ||
		full["purchasePlan"] != "plan-buy-1" || full["agreementScope"] != "scope-procurement" {
		t.Fatalf("带正文的行没有逐字段透出:%v", full)
	}
	if full["agreementEffectiveEndsAt"] == nil || full["registeredAt"] == nil {
		t.Fatalf("正文区间与登记时刻没透出:%v", full)
	}
	if _, has := full["direction"]; has {
		t.Fatal("行体长出了方向键——方向恒为 BUY，不转写常量")
	}

	shell := rows[1].(map[string]any)
	if shell["contentRegistered"] != false {
		t.Fatalf("只有壳的行 contentRegistered = %v", shell["contentRegistered"])
	}
	for _, key := range []string{"supplier", "legalEntity", "purchasePlan", "agreementScope",
		"agreementEffectiveStartsAt", "agreementEffectiveEndsAt", "registeredAt"} {
		if _, has := shell[key]; has {
			t.Fatalf("只有壳的行长出了正文键 %q:%v", key, shell)
		}
	}
	if shell["scope"] != "scope-1" || shell["status"] != "EFFECTIVE" {
		t.Fatalf("壳字段没有照列:%v", shell)
	}
}
