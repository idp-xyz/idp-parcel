package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func sellPriceSources(t *testing.T, names ...string) *domain.SellPriceArrangementBody {
	t.Helper()
	return &domain.SellPriceArrangementBody{Sources: pricingScopes(t, names...)}
}

func noSellPrice(t *testing.T, basis string) *domain.SellPriceArrangementBody {
	t.Helper()
	return &domain.SellPriceArrangementBody{NoSellPriceBasis: commercialValue(t, domain.NewNoSellPriceBasis, basis)}
}

func contractBodyWithSellPrice(t *testing.T, arrangement *domain.SellPriceArrangementBody) domain.CustomerContractBody {
	t.Helper()
	body := customerContractBody(t, notApplicableControl(t),
		appliedControl(t, "charge-prepaid"), inapplicableControl(t, "charge-cod", "COD-NA-01"))
	body.SellPrice = arrangement
	return body
}

// Covers: ADR-0380 决定七「合同正文加键是一次形状变更，换规范化版本号」与 ADR-0267 决定一——带销售价格约定的
// 合同文档写成 PCC-3；来源按声明的顺序写出、不排序（顺序是正文），所以换顺序、换范围、换成不计销售价各是另一个串。
func TestAContractWithASellPriceArrangementIsCanonicalizedAsPCC3(t *testing.T) {
	ordered := canonicalCustomerContract(t, contractBodyWithSellPrice(t, sellPriceSources(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD")))
	if ordered.Canonicalization() != "PCC-3" || !strings.HasPrefix(ordered.Digest().String(), "PCC-3:") {
		t.Fatalf("a contract with sell price sources must be canonicalized under PCC-3, got %s", ordered.Digest())
	}

	var document struct {
		Canonicalization string `json:"canonicalization"`
		CustomerContract struct {
			SellPriceArrangement struct {
				Sources          []map[string]string `json:"sources"`
				NoSellPriceBasis *string             `json:"noSellPriceBasis"`
			} `json:"sellPriceArrangement"`
		} `json:"customerContract"`
	}
	if err := json.Unmarshal(ordered.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", ordered.Document(), err)
	}
	sources := document.CustomerContract.SellPriceArrangement.Sources
	if document.Canonicalization != "PCC-3" || len(sources) != 2 ||
		sources[0]["pricingScope"] != "SYN-SCOPE-EXCLUSIVE" || sources[1]["pricingScope"] != "SYN-SCOPE-STANDARD" {
		t.Fatalf("document = %s", ordered.Document())
	}
	if document.CustomerContract.SellPriceArrangement.NoSellPriceBasis != nil {
		t.Fatalf("a priced contract must omit noSellPriceBasis: %s", ordered.Document())
	}

	again := canonicalCustomerContract(t, contractBodyWithSellPrice(t, sellPriceSources(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD")))
	if again.Digest() != ordered.Digest() {
		t.Fatalf("same body produced two digests: %s / %s", ordered.Digest(), again.Digest())
	}
	reordered := canonicalCustomerContract(t, contractBodyWithSellPrice(t, sellPriceSources(t, "SYN-SCOPE-STANDARD", "SYN-SCOPE-EXCLUSIVE")))
	if reordered.Digest() == ordered.Digest() {
		t.Fatal("swapping the order of the sell price sources must change the digest")
	}
	single := canonicalCustomerContract(t, contractBodyWithSellPrice(t, sellPriceSources(t, "SYN-SCOPE-STANDARD")))
	if single.Digest() == ordered.Digest() || single.Digest() == reordered.Digest() {
		t.Fatal("dropping a source must change the digest")
	}

	notPriced := canonicalCustomerContract(t, contractBodyWithSellPrice(t, noSellPrice(t, "SYN-NO-SELL-PRICE-01")))
	if notPriced.Canonicalization() != "PCC-3" || notPriced.Digest() == single.Digest() {
		t.Fatalf("an explicit no-sell-price contract must be its own PCC-3 digest, got %s", notPriced.Digest())
	}
	if !strings.Contains(string(notPriced.Document()), `"sellPriceArrangement":{"noSellPriceBasis":"SYN-NO-SELL-PRICE-01"}`) {
		t.Fatalf("a not-priced contract carries only the basis: %s", notPriced.Document())
	}
}

// Covers: ADR-0267 决定一「不带新内容的文档仍写旧版、逐字节不变」与票面完成判据「旧规范化版本的合同读回不变，
// 摘要不变」——没有销售价格约定的合同仍是 PCC-1。期望串取自加这一格之前的构建对同一份正文算出的字节。
func TestAContractWithoutASellPriceArrangementStaysByteIdenticalUnderPCC1(t *testing.T) {
	const documentBefore = `{"canonicalization":"PCC-1","kind":"CUSTOMER_CONTRACT","customerContract":{"contractContent":{"customerAccount":"customer-1","rulePackage":"rules-1","bindings":[{"chargeScope":"charge-cod","inapplicabilityBasis":"COD-NA-01"},{"chargeScope":"charge-prepaid","policy":"policy-charge-prepaid"}]},"preAcceptanceControl":{"requirement":"NOT_APPLICABLE","notApplicableBasis":"CONTRACT-CLAUSE/NO-PRE-ACCEPTANCE-CONTROL"}}}`
	const digestBefore = "PCC-1:fbdfc2490538167cec4393df722fefc4a024e5509faca27e13531633dcec2751"

	canonical := canonicalCustomerContract(t, contractBodyWithSellPrice(t, nil))
	if string(canonical.Document()) != documentBefore {
		t.Fatalf("document changed:\n got %s\nwant %s", canonical.Document(), documentBefore)
	}
	if canonical.Digest().String() != digestBefore || canonical.Canonicalization() != "PCC-1" {
		t.Fatalf("digest = %s, want %s", canonical.Digest(), digestBefore)
	}
}

// Covers: ADR-0126 Decision 三（正文快照）——PCC-3 的合同文档折得回正文：来源按原顺序回到领域值对象、不计销售价
// 带着依据回来，折回去再算一遍与列里的摘要相等。标签与形状对不上的文档不是本构建写下的（ADR-0014）：PCC-1 的
// 文档标成 PCC-3、带着这一节的文档标成 PCC-1、别的册标成 PCC-3，都答不认这一版，不猜着折。
func TestAContractDocumentWithASellPriceArrangementRehydratesToTheSameBody(t *testing.T) {
	ordered := canonicalCustomerContract(t, contractBodyWithSellPrice(t, sellPriceSources(t, "SYN-SCOPE-EXCLUSIVE", "SYN-SCOPE-STANDARD")))
	content, err := domain.RehydratePublicationContent(ordered.Canonicalization(), ordered.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.CustomerContract == nil || content.CustomerContract.SellPrice == nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	sources := content.CustomerContract.SellPrice.Sources
	if len(sources) != 2 || sources[0].String() != "SYN-SCOPE-EXCLUSIVE" || sources[1].String() != "SYN-SCOPE-STANDARD" ||
		content.CustomerContract.SellPrice.NoSellPriceBasis.String() != "" {
		t.Fatalf("sell price arrangement = %#v", content.CustomerContract.SellPrice)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil || recomputed.Digest() != ordered.Digest() {
		t.Fatalf("rehydrated body digests to %s (%v), column says %s", recomputed.Digest(), err, ordered.Digest())
	}

	notPriced := canonicalCustomerContract(t, contractBodyWithSellPrice(t, noSellPrice(t, "SYN-NO-SELL-PRICE-01")))
	content, err = domain.RehydratePublicationContent(notPriced.Canonicalization(), notPriced.Document())
	if err != nil {
		t.Fatalf("rehydrate not priced: %v", err)
	}
	if arrangement := content.CustomerContract.SellPrice; arrangement == nil || len(arrangement.Sources) != 0 ||
		arrangement.NoSellPriceBasis.String() != "SYN-NO-SELL-PRICE-01" {
		t.Fatalf("not-priced arrangement = %#v", content.CustomerContract.SellPrice)
	}

	without := canonicalCustomerContract(t, contractBodyWithSellPrice(t, nil))
	content, err = domain.RehydratePublicationContent(without.Canonicalization(), without.Document())
	if err != nil || content.CustomerContract == nil || content.CustomerContract.SellPrice != nil {
		t.Fatalf("an absent arrangement must rehydrate to nil, got %#v (%v)", content.CustomerContract, err)
	}

	relabel := func(document []byte, from, to string) []byte {
		return []byte(strings.Replace(string(document), `"canonicalization":"`+from+`"`, `"canonicalization":"`+to+`"`, 1))
	}
	if _, err := domain.RehydratePublicationContent("PCC-3", relabel(without.Document(), "PCC-1", "PCC-3")); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("a PCC-1 shaped contract labelled PCC-3: err = %v, want ErrCanonicalizationUnsupported", err)
	}
	if _, err := domain.RehydratePublicationContent("PCC-1", relabel(ordered.Document(), "PCC-3", "PCC-1")); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("a contract carrying sell price sources labelled PCC-1: err = %v, want ErrCanonicalizationUnsupported", err)
	}
	creditPolicy := `{"canonicalization":"PCC-3","kind":"CREDIT_POLICY","creditPolicy":{"legalEntity":"le-1","authorityLevel":"L1","chargeType":"freight","limitMinor":100,"effectiveStartsAt":"2026-01-01T00:00:00Z"}}`
	if _, err := domain.RehydratePublicationContent("PCC-3", []byte(creditPolicy)); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("another register labelled PCC-3: err = %v, want ErrCanonicalizationUnsupported", err)
	}

	// 快照里坏一格（同一范围两次）折不回：快照是数据，约定立不立得住仍由构造门说。
	corrupted := strings.Replace(string(ordered.Document()), `"pricingScope":"SYN-SCOPE-STANDARD"`, `"pricingScope":"SYN-SCOPE-EXCLUSIVE"`, 1)
	if _, err := domain.RehydratePublicationContent("PCC-3", []byte(corrupted)); !errors.Is(err, domain.ErrDuplicateSellPriceSource) {
		t.Fatalf("corrupted snapshot: err = %v, want ErrDuplicateSellPriceSource", err)
	}
}

// Covers: ADR-0380 决定一——折成文档前过的是与绑定到版本上同一道门：约定在场却两种都没声明、两种都声明、
// 同一范围两次，都折不出摘要。
func TestSellPriceArrangementCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	both := sellPriceSources(t, "SYN-SCOPE-STANDARD")
	both.NoSellPriceBasis = commercialValue(t, domain.NewNoSellPriceBasis, "SYN-NO-SELL-PRICE-01")
	refusals := map[string]struct {
		arrangement *domain.SellPriceArrangementBody
		want        error
	}{
		"约定在场却两种都没声明": {arrangement: &domain.SellPriceArrangementBody{}, want: domain.ErrInvalidSellPriceArrangement},
		"两种都声明":       {arrangement: both, want: domain.ErrInvalidSellPriceArrangement},
		"同一范围出现两次": {
			arrangement: sellPriceSources(t, "SYN-SCOPE-STANDARD", "SYN-SCOPE-STANDARD"),
			want:        domain.ErrDuplicateSellPriceSource,
		},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := contractBodyWithSellPrice(t, refusal.arrangement)
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject, CustomerContract: &body})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}
