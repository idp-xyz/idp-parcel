package commercialhttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对客户合同版本的销售价格约定（ADR-0380 决定一；票 commercial-four-module-requirements/16）证传输面的两半：
// 运营操作者面载荷里的那一节（键名镜像受控批文，来源照录入的顺序到达），与客户合同目录答复里读回的那一节。

func contractPayloadWithSellPrice(arrangement string) string {
	return fmt.Sprintf(`{
	  "kind": "CUSTOMER_CONTRACT",
	  "objectId": "contract-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "customerContract": {
	    "contractContent": {"customerAccount": "customer-1", "rulePackage": "rules-1"},
	    "sellPriceArrangement": %s
	  }
	}`, arrangement)
}

// Covers: 票面「录入…面带上这一格」——载荷里的来源照录入的顺序折成领域正文、不排序；预览按含这一层的正文算出
// PCC-3 摘要，与直接用领域值对象算出的逐字节相同；不计销售价只带依据；整节缺席的合同照旧是 PCC-1，不补默认。
func TestCustomerContractPayloadCarriesTheSellPriceArrangementInTheSubmittedOrder(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	raw := contractPayloadWithSellPrice(`{"sources": [{"pricingScope": "SYN-SCOPE-Z"}, {"pricingScope": "SYN-SCOPE-A"}]}`)
	_, content, err := decodePublication(t, raw).Publication(tenant)
	if err != nil {
		t.Fatalf("publication: %v", err)
	}
	arrangement := content.CustomerContract.SellPrice
	if arrangement == nil || len(arrangement.Sources) != 2 ||
		arrangement.Sources[0].String() != "SYN-SCOPE-Z" || arrangement.Sources[1].String() != "SYN-SCOPE-A" ||
		arrangement.NoSellPriceBasis.String() != "" {
		t.Fatalf("sell price arrangement = %#v, want the submitted order", arrangement)
	}

	previewCommand, err := decodePublication(t, raw).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v (%v)", preview.Outcome(), err, preview.RefusalCause())
	}
	previewed, _ := preview.Canonical()
	direct, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if previewed.Digest() != direct.Digest() || !strings.HasPrefix(previewed.Digest().String(), "PCC-3:") {
		t.Fatalf("preview digest %s ≠ domain digest %s, or not PCC-3", previewed.Digest(), direct.Digest())
	}

	_, content, err = decodePublication(t, contractPayloadWithSellPrice(`{"noSellPriceBasis": "SYN-NO-SELL-PRICE-01"}`)).Publication(tenant)
	if err != nil {
		t.Fatalf("publication (not priced): %v", err)
	}
	if notPriced := content.CustomerContract.SellPrice; notPriced == nil || len(notPriced.Sources) != 0 ||
		notPriced.NoSellPriceBasis.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("not-priced arrangement = %#v", content.CustomerContract.SellPrice)
	}

	_, content, err = decodePublication(t, customerContractPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("publication (no arrangement): %v", err)
	}
	if content.CustomerContract.SellPrice != nil {
		t.Fatalf("没给这一节却折出了一份约定：%#v", content.CustomerContract.SellPrice)
	}
}

// Covers: 逐格问题——两个键都给或都不给在这一节点名（恰一，问题码 EXACTLY_ONE），不挑一格读，也不把都没给折成
// 「不计销售价」；某一项的计价范围留白记在那一项自己的格上（REQUIRED）；成立的格不被点名。
func TestCustomerContractPayloadPointsAtSellPriceArrangementProblems(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	problemsOf := func(t *testing.T, arrangement string) map[string]string {
		t.Helper()
		_, _, err := decodePublication(t, contractPayloadWithSellPrice(arrangement)).Publication(tenant)
		var problems *commercialhttp.PublicationPayloadProblems
		if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("err = %v, want *PublicationPayloadProblems", err)
		}
		codes := map[string]string{}
		for _, problem := range problems.Problems {
			codes[problem.Field] = problem.Code
		}
		return codes
	}

	for name, arrangement := range map[string]string{
		"两个键都给":  `{"sources": [{"pricingScope": "SYN-SCOPE-A"}], "noSellPriceBasis": "SYN-NO-SELL-PRICE-01"}`,
		"两个键都没给": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			codes := problemsOf(t, arrangement)
			if len(codes) != 1 || codes["customerContract.sellPriceArrangement"] != commercialhttp.ProblemCodeExactlyOne {
				t.Fatalf("problems = %v, want EXACTLY_ONE on customerContract.sellPriceArrangement alone", codes)
			}
		})
	}

	blank := problemsOf(t, `{"sources": [{"pricingScope": "SYN-SCOPE-A"}, {"pricingScope": "  "}]}`)
	if len(blank) != 1 || blank["customerContract.sellPriceArrangement.sources[1].pricingScope"] != commercialhttp.ProblemCodeRequired {
		t.Fatalf("problems = %v, want REQUIRED on sources[1].pricingScope alone", blank)
	}
}

// Covers: 同一计价范围出现两次是跨项的问题，解码不点名，由领域在预览上答成`未受理`带成因（判据同约定表的同一
// 范围两行）。
func TestASellPriceSourceNamedTwiceIsRefusedByThePreview(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	raw := contractPayloadWithSellPrice(`{"sources": [{"pricingScope": "SYN-SCOPE-A"}, {"pricingScope": "SYN-SCOPE-A"}]}`)
	previewCommand, err := decodePublication(t, raw).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	answer, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || answer.Outcome() != application.CommercialPublicationPreviewNotAccepted ||
		!errors.Is(answer.RefusalCause(), domain.ErrDuplicateSellPriceSource) {
		t.Fatalf("preview = %q, %v (cause %v); want NOT_ACCEPTED / ErrDuplicateSellPriceSource", answer.Outcome(), err, answer.RefusalCause())
	}
}

// Covers: 票面「读面带出这一格」——客户合同目录答复逐行带销售价格约定：来源按合同声明的顺序、每项一个 pricingScope；
// 不计销售价的约定带依据而来源为空数组；没有这一层的合同不长这一键，不拿空列表顶。
func TestCustomerContractsEndpointEchoesTheSellPriceArrangement(t *testing.T) {
	query := catalogueQuery(t)
	shell := func(objectID string) ports.CustomerContractCatalogueRow {
		return ports.CustomerContractCatalogueRow{ObjectID: objectID, VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
			EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt,
			HasContent: true, RulePackageID: "rules-1", CustomerAccountID: "account-1", DeclaredAt: catBaseAt}
	}
	priced, notPriced, silent := shell("contract-1"), shell("contract-2"), shell("contract-3")
	priced.SellPrice = &ports.SellPriceArrangementCatalogueRow{Sources: []string{"SYN-SCOPE-Z", "SYN-SCOPE-A"}, DeclaredAt: catBaseAt}
	notPriced.SellPrice = &ports.SellPriceArrangementCatalogueRow{Sources: []string{}, NoSellPriceBasis: "SYN-NO-SELL-PRICE-01", DeclaredAt: catBaseAt}
	reader := &relationReaderDouble{
		tenant:    query.Scope.Tenant(),
		contracts: []ports.CustomerContractCatalogueRow{priced, notPriced, silent},
	}
	endpoint := commercialhttp.NewQueryCustomerContractsEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-customer-contracts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	rows, _ := decodeBody(t, recorder)["contracts"].([]any)
	if len(rows) != 3 {
		t.Fatalf("contracts = %v", rows)
	}

	first, _ := rows[0].(map[string]any)["sellPriceArrangement"].(map[string]any)
	sources, _ := first["sources"].([]any)
	if len(sources) != 2 ||
		sources[0].(map[string]any)["pricingScope"] != "SYN-SCOPE-Z" || sources[1].(map[string]any)["pricingScope"] != "SYN-SCOPE-A" {
		t.Fatalf("有来源的约定 = %v，want 合同声明的顺序", first)
	}
	if _, has := first["noSellPriceBasis"]; has || first["declaredAt"] == nil {
		t.Fatalf("有来源的约定长出了依据键或缺声明时刻：%v", first)
	}

	second, _ := rows[1].(map[string]any)["sellPriceArrangement"].(map[string]any)
	if empty, isArray := second["sources"].([]any); !isArray || len(empty) != 0 || second["noSellPriceBasis"] != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("不计销售价的约定 = %v", second)
	}

	if _, has := rows[2].(map[string]any)["sellPriceArrangement"]; has {
		t.Fatalf("没有销售价格约定的合同长出了这一键：%v", rows[2])
	}
}
