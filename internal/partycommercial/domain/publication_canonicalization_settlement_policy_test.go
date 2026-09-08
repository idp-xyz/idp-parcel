package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证结算政策册接进服务端规范化（票 admin-write-faces/15；ADR-0126 Decision 一「加册不换号」）：
// 同一份正文逐字节同摘要、文档键名镜像受控批文的 settlementPolicyBody、合同维写领域的两段式指称串、
// 三格拒绝各归其格、文档能折回正文、结算方式的反查与 String() 互逆。

func settlementApplicabilityFixture(t *testing.T, contractObject, contractVersion, currency string, endsAt time.Time) domain.SettlementApplicability {
	t.Helper()
	contract, err := domain.NewQualifiedVersionLabel(
		commercialValue(t, domain.NewCommercialObjectID, contractObject),
		commercialValue(t, domain.NewCommercialVersionLabel, contractVersion),
	)
	if err != nil {
		t.Fatalf("qualified contract label: %v", err)
	}
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	applicability, err := domain.NewSettlementApplicability(
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCounterpartyReference, "account-1"),
		contract,
		commercialValue(t, domain.NewChargeScopeReference, "charge-prepaid"),
		commercialValue(t, domain.NewCurrencyCode, currency),
		interval,
	)
	if err != nil {
		t.Fatalf("settlement applicability: %v", err)
	}
	return applicability
}

func settlementPolicyBody(t *testing.T, method domain.SettlementMethod, currency string, endsAt time.Time) domain.SettlementPolicyBody {
	t.Helper()
	return domain.SettlementPolicyBody{
		Method:        method,
		Applicability: settlementApplicabilityFixture(t, "contract-1", "v1", currency, endsAt),
	}
}

func canonicalSettlementPolicy(t *testing.T, body domain.SettlementPolicyBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:             domain.SettlementPolicyObject,
		SettlementPolicy: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一 — 结算政策接进 PCC-1 不换号；同一正文两次算逐字节同串；文档键名镜像批文
// settlementPolicyBodyDocument（method / legalEntity / counterparty / contract / chargeScope / currency / effectiveStartsAt /
// effectiveEndsAt）；合同维写的是领域的两段式指称串「对象/版本」（NewQualifiedVersionLabel 那一处拼的、0011 的
// contract_label 存的那一个），不是批文里对象 + 版本两格；区间无上界时 effectiveEndsAt 缺席。
func TestSettlementPolicyCanonicalizesIntoTheSameVersionMirroringTheBatchDocument(t *testing.T) {
	body := settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Time{})
	first := canonicalSettlementPolicy(t, body)
	second := canonicalSettlementPolicy(t, body)

	if first.Digest() != second.Digest() {
		t.Fatalf("same body canonicalized twice: %s vs %s", first.Digest(), second.Digest())
	}
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("canonicalization = %q, digest = %q; want PCC-1", first.Canonicalization(), first.Digest())
	}
	// 文档字节整份钉死：键名与顺序就是规范化的全部，改一处即另一个版本号（ADR-0014）。
	wantDocument := `{"canonicalization":"PCC-1","kind":"SETTLEMENT_POLICY",` +
		`"settlementPolicy":{"method":"PREPAID","legalEntity":"legal-1","counterparty":"account-1",` +
		`"contract":"contract-1/v1","chargeScope":"charge-prepaid","currency":"CNY","effectiveStartsAt":"2026-01-03T00:00:00Z"}}`
	if got := string(first.Document()); got != wantDocument {
		t.Fatalf("document = %s\nwant       %s", got, wantDocument)
	}

	bounded := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))
	if !strings.Contains(string(bounded.Document()), `"effectiveEndsAt":"2026-06-30T16:00:00Z"`) {
		t.Fatalf("bounded interval did not write its end: %s", bounded.Document())
	}
	if bounded.Digest() == first.Digest() {
		t.Fatal("bounded and open-ended intervals must not share a digest")
	}
}

// Covers: ADR-0126 Decision 一 — 方式或六维任一格不同即另一个串（换方式、换币种、换合同版本）；同一时刻不同
// 时区写法不产生第二个串。合同维要换版本也变串：结算约定属于哪一版客户合同是正文的一部分，不是可省的修饰。
func TestSettlementPolicyDigestDistinguishesContentButNotSpelling(t *testing.T) {
	prepaid := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Time{}))
	terms := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.TermsMethod, "CNY", time.Time{}))
	if prepaid.Digest() == terms.Digest() {
		t.Fatal("PREPAID and TERMS must not share a digest")
	}
	usd := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.PrepaidMethod, "USD", time.Time{}))
	if prepaid.Digest() == usd.Digest() {
		t.Fatal("two currencies must not share a digest")
	}
	otherContractVersion := canonicalSettlementPolicy(t, domain.SettlementPolicyBody{
		Method:        domain.PrepaidMethod,
		Applicability: settlementApplicabilityFixture(t, "contract-1", "v2", "CNY", time.Time{}),
	})
	if prepaid.Digest() == otherContractVersion.Digest() {
		t.Fatal("two contract versions must not share a digest")
	}

	shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
	inUTC := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))
	inShanghai := canonicalSettlementPolicy(t, settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Date(2026, 7, 1, 0, 0, 0, 0, shanghai)))
	if inUTC.Digest() != inShanghai.Digest() {
		t.Fatalf("same instant in two zones produced two digests: %s vs %s", inUTC.Digest(), inShanghai.Digest())
	}
}

// Covers: ADR-0126 Decision 一 — 三格分开：正文缺席、正文与类别不符（两个方向）、零值正文与方式集外的正文；
// 本册从此算「已接」。
func TestSettlementPolicyCanonicalizationRefusals(t *testing.T) {
	body := settlementPolicyBody(t, domain.PrepaidMethod, "CNY", time.Time{})
	credit := creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{})

	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.SettlementPolicyObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("settlement policy without body: err = %v, want ErrPublicationContentAbsent", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:             domain.CreditPolicyObject,
		SettlementPolicy: &body,
	})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("settlement body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:         domain.SettlementPolicyObject,
		CreditPolicy: &credit,
	})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("credit body under settlement kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:             domain.SettlementPolicyObject,
		SettlementPolicy: &domain.SettlementPolicyBody{},
	})
	if !errors.Is(err, domain.ErrInvalidSettlementPolicy) {
		t.Fatalf("zero body: err = %v, want ErrInvalidSettlementPolicy", err)
	}
	// 方式是封闭两值：第三个取值（客户级默认）在领域被排除过一次，文档层不得替它开口。
	methodless := domain.SettlementPolicyBody{Method: domain.SettlementMethodInvalid, Applicability: body.Applicability}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:             domain.SettlementPolicyObject,
		SettlementPolicy: &methodless,
	})
	if !errors.Is(err, domain.ErrInvalidSettlementPolicy) {
		t.Fatalf("body without a method: err = %v, want ErrInvalidSettlementPolicy", err)
	}
	if !domain.IsRegisterCanonicalized(domain.SettlementPolicyObject) {
		t.Fatal("IsRegisterCanonicalized: settlement policy is canonicalized by this build")
	}
}

// Covers: ADR-0126 Decision 三 — 载体快照就是规范化文档：结算政策文档折回正文（方式与六维原样、合同维仍是同一个
// 两段式串）、再算同摘要；只有壳的文档折不回；方式集外或某维为空的文档过不了构造门。
func TestSettlementPolicyDocumentRoundTripsTheContent(t *testing.T) {
	body := settlementPolicyBody(t, domain.TermsMethod, "CNY", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC))
	canonical := canonicalSettlementPolicy(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate content: %v", err)
	}
	if content.Kind != domain.SettlementPolicyObject || content.SettlementPolicy == nil || content.CreditPolicy != nil {
		t.Fatalf("content = %#v", content)
	}
	if content.SettlementPolicy.Method != domain.TermsMethod {
		t.Fatalf("method = %s, want TERMS", content.SettlementPolicy.Method)
	}
	if content.SettlementPolicy.Applicability != body.Applicability {
		t.Fatalf("applicability did not travel: %#v", content.SettlementPolicy.Applicability)
	}
	if content.SettlementPolicy.Applicability.Contract().String() != "contract-1/v1" {
		t.Fatalf("contract label = %q", content.SettlementPolicy.Applicability.Contract())
	}
	again, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("re-canonicalize: %v", err)
	}
	if again.Digest() != canonical.Digest() {
		t.Fatalf("round trip changed the digest: %s vs %s", again.Digest(), canonical.Digest())
	}

	shellOnly := []byte(`{"canonicalization":"PCC-1","kind":"SETTLEMENT_POLICY"}`)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), shellOnly); !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("a document without its body: err = %v, want ErrPublicationContentAbsent", err)
	}
	documentWith := func(method, counterparty string) []byte {
		return []byte(`{"canonicalization":"PCC-1","kind":"SETTLEMENT_POLICY","settlementPolicy":{"method":"` + method +
			`","legalEntity":"legal-1","counterparty":"` + counterparty +
			`","contract":"contract-1/v1","chargeScope":"charge-prepaid","currency":"CNY","effectiveStartsAt":"2026-01-03T00:00:00Z"}}`)
	}
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), documentWith("CASH", "account-1")); err == nil {
		t.Fatal("a method outside the closed set must not rehydrate")
	}
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), documentWith("PREPAID", " ")); err == nil {
		t.Fatal("a blank counterparty must not rehydrate")
	}
}

// Covers: SettlementMethodNamed 与 String() 互逆——名单只在 String() 一处，反查不另抄；集合外与空串答 false。
func TestSettlementMethodNamedReversesString(t *testing.T) {
	for _, method := range []domain.SettlementMethod{domain.PrepaidMethod, domain.TermsMethod} {
		named, known := domain.SettlementMethodNamed(method.String())
		if !known || named != method {
			t.Fatalf("SettlementMethodNamed(%q) = (%v, %v), want (%v, true)", method.String(), named, known, method)
		}
	}
	for _, outside := range []string{"", "CASH", "prepaid", "PREPAID "} {
		if _, known := domain.SettlementMethodNamed(outside); known {
			t.Fatalf("SettlementMethodNamed(%q) must not be known", outside)
		}
	}
}
