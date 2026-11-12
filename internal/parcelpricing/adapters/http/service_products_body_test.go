package pricinghttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件证适用服务产品（ADR-0382 决定一）在答复体里的写法：导入预览与草稿共用的方案概要、价卡目录行都交 serviceProducts，
// 同名同形（字符串数组），不声明即缺键——缺键就是接其余产品。

func namedEstimatePlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	product, err := domain.NewServiceProductReference("SYN-PROD-FBA")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := estimatePlan(t).WithServiceProducts(product)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestThePreviewContentEchoesTheServiceProducts(t *testing.T) {
	authorization := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH", "v1")
	for name, test := range map[string]struct {
		plan domain.PricingPlanVersion
		want string
	}{
		"指名":  {namedEstimatePlan(t), `"serviceProducts":["SYN-PROD-FBA"]`},
		"不声明": {estimatePlan(t), ""},
	} {
		recorder := postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{preview: application.PriceCardImportPreview{
			Outcome: application.PriceCardImportValidated, TemplateVersion: "PPT-3", Plan: test.plan.Reference(),
			Content: &ports.PriceCardTemplateContent{Plan: test.plan, DirectionAuthorization: authorization},
		}})
		body := recorder.Body.String()
		if test.want != "" && !strings.Contains(body, test.want) {
			t.Fatalf("%s：预览没交出适用产品：%s", name, body)
		}
		if test.want == "" && strings.Contains(body, "serviceProducts") {
			t.Fatalf("%s：不声明的卡长出了 serviceProducts 键：%s", name, body)
		}
	}
}

func TestThePriceCardListEchoesTheServiceProducts(t *testing.T) {
	query := catalogueQuery(t)
	reader := &cardReaderDouble{tenant: query.Scope.Tenant(), rows: []ports.PriceCardCatalogueRow{
		{PlanID: "plan-fba", PlanVersion: "v1", Direction: "SELL", Purpose: "CUSTOMER_CHARGE", Scope: "scope-1",
			EffectiveFrom: pricingBaseAt, RegisteredAt: pricingBaseAt, ServiceProducts: []string{"SYN-PROD-EXPRESS", "SYN-PROD-FBA"}},
		{PlanID: "plan-rest", PlanVersion: "v1", Direction: "SELL", Purpose: "CUSTOMER_CHARGE", Scope: "scope-1",
			EffectiveFrom: pricingBaseAt, RegisteredAt: pricingBaseAt},
	}}
	recorder := httptest.NewRecorder()
	pricinghttp.NewQueryPriceCardsEndpoint(intakeDouble{query: query}, reader).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil))
	cards := decodeBody(t, recorder)["cards"].([]any)
	first, second := cards[0].(map[string]any), cards[1].(map[string]any)
	products, ok := first["serviceProducts"].([]any)
	if !ok || len(products) != 2 || products[0] != "SYN-PROD-EXPRESS" || products[1] != "SYN-PROD-FBA" {
		t.Fatalf("指名卡的 serviceProducts = %v", first["serviceProducts"])
	}
	if _, has := second["serviceProducts"]; has {
		t.Fatalf("不声明的卡长出了 serviceProducts 键：%v", second)
	}
}
