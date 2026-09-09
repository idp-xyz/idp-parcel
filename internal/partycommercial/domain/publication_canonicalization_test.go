package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func creditPolicyBody(t *testing.T, chargeType string, limit domain.CreditLimit, endsAt time.Time) domain.CreditPolicyBody {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.CreditPolicyBody{
		LegalEntity: commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		Level:       commercialValue(t, domain.NewAuthorityLevel, "level-commercial"),
		ChargeType:  commercialValue(t, domain.NewChargeTypeReference, chargeType),
		Limit:       limit,
		Effective:   interval,
	}
}

func canonicalCreditPolicy(t *testing.T, body domain.CreditPolicyBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:         domain.CreditPolicyObject,
		CreditPolicy: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一 — 摘要串自带规范化版本，同一正文两次算逐字节同串，形如 PCC-1:<hex>。
func TestCanonicalDigestCarriesVersionAndIsStable(t *testing.T) {
	body := creditPolicyBody(t, "freight", creditAmount(t, 500_000), time.Time{})
	first := canonicalCreditPolicy(t, body)
	second := canonicalCreditPolicy(t, body)

	if first.Digest() != second.Digest() {
		t.Fatalf("same body canonicalized twice: %s vs %s", first.Digest(), second.Digest())
	}
	// 版本号按 ADR-0126 Decision 一的原文写死：拿代码里的常量来比是同义反复，形状变了测试也不会响。
	if first.Canonicalization() != "PCC-1" {
		t.Fatalf("canonicalization = %q, want PCC-1", first.Canonicalization())
	}
	if !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("digest %q does not carry its canonicalization version", first.Digest())
	}
	if hexLength := len(strings.TrimPrefix(first.Digest().String(), "PCC-1:")); hexLength != 64 {
		t.Fatalf("digest hex length = %d, want 64 (sha256)", hexLength)
	}
	carried, ok := first.Digest().Canonicalization()
	if !ok || carried != "PCC-1" {
		t.Fatalf("Canonicalization() = %q, %v; want PCC-1, true", carried, ok)
	}
}

// Covers: ADR-0126 Decision 一 — 正文任一格不同即另一个串：额度落在金额格与比例格、同一时刻不同时区
// 写法不产生第二个串（UTC 归一）。
func TestCanonicalDigestDistinguishesContentButNotSpelling(t *testing.T) {
	amount := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{}))
	ratio, err := domain.NewCreditRatioLimit(100, domain.PostedBalanceBase)
	if err != nil {
		t.Fatalf("new ratio limit: %v", err)
	}
	asRatio := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", ratio, time.Time{}))
	if amount.Digest() == asRatio.Digest() {
		t.Fatalf("amount 100 and ratio 100 must not share a digest")
	}
	// ADR-0129：同一个比例相对不同基数是两份不同的授权，摘要必须分开；基数键随比例进 PCC-1 而不换号。
	onPriorPeriod, err := domain.NewCreditRatioLimit(100, domain.PriorPeriodConfirmedChargesBase)
	if err != nil {
		t.Fatalf("new ratio limit: %v", err)
	}
	asRatioOnPriorPeriod := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", onPriorPeriod, time.Time{}))
	if asRatio.Digest() == asRatioOnPriorPeriod.Digest() {
		t.Fatalf("ratio 100 on POSTED_BALANCE and on PRIOR_PERIOD_CONFIRMED_CHARGES must not share a digest")
	}
	if asRatio.Canonicalization() != "PCC-1" || asRatioOnPriorPeriod.Canonicalization() != "PCC-1" {
		t.Fatalf("ratio base joined the document under a new canonicalization version: %s / %s",
			asRatio.Canonicalization(), asRatioOnPriorPeriod.Canonicalization())
	}
	otherCharge := canonicalCreditPolicy(t, creditPolicyBody(t, "surcharge", creditAmount(t, 100), time.Time{}))
	if amount.Digest() == otherCharge.Digest() {
		t.Fatalf("different charge type must not share a digest")
	}

	shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
	inUTC := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", creditAmount(t, 100),
		time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))
	inShanghai := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", creditAmount(t, 100),
		time.Date(2026, 7, 1, 0, 0, 0, 0, shanghai)))
	if inUTC.Digest() != inShanghai.Digest() {
		t.Fatalf("same instant in two zones produced two digests: %s vs %s", inUTC.Digest(), inShanghai.Digest())
	}
}

// Covers: ADR-0126 Decision 一 — 三格分开：没接的册、已接的册正文缺席、正文与类别不符。
func TestCanonicalizeAnswersThreeDistinctRefusals(t *testing.T) {
	body := creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{})

	// 「没接的册」自票 admin-write-faces/18 起没有样本：封闭集里的每一册都接进了 PCC-1，那一格对任何合法类别都答不出来。
	// 这里改钉「全接」——某册被剪出规范化时这里红，而不是让 ErrRegisterNotCanonicalized 悄悄重新有了实例。
	for _, kind := range allCommercialObjectKinds {
		if !domain.IsRegisterCanonicalized(kind) {
			t.Fatalf("%s: IsRegisterCanonicalized answers false; every register in the closed set is wired since ticket 18", kind)
		}
		if _, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: kind}); errors.Is(err, domain.ErrRegisterNotCanonicalized) {
			t.Fatalf("%s: a wired register answered ErrRegisterNotCanonicalized", kind)
		}
	}
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CreditPolicyObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("credit policy without body: err = %v, want ErrPublicationContentAbsent", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:         domain.SettlementPolicyObject,
		CreditPolicy: &body,
	})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("credit body under settlement kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:         domain.CreditPolicyObject,
		CreditPolicy: &domain.CreditPolicyBody{},
	})
	if !errors.Is(err, domain.ErrInvalidCreditPolicy) {
		t.Fatalf("zero body: err = %v, want ErrInvalidCreditPolicy", err)
	}
	if domain.IsRegisterCanonicalized(domain.CommercialObjectKindInvalid) {
		t.Fatalf("IsRegisterCanonicalized: a kind outside the closed set must not read as canonicalized")
	}
}

// allCommercialObjectKinds 是封闭集的全部十格，与 TestCommercialObjectKindsStayIndependentAndClosed 列的同一份。
var allCommercialObjectKinds = []domain.CommercialObjectKind{
	domain.ServiceProductObject,
	domain.CustomerContractObject,
	domain.SupplierAgreementObject,
	domain.AcceptanceRulePackageObject,
	domain.PreAcceptanceFinancialControlPolicyObject,
	domain.PriceRuleObject,
	domain.SettlementPolicyObject,
	domain.CreditPolicyObject,
	domain.AuthorizationRuleObject,
	domain.CustomerServiceRuleObject,
}

// Covers: ADR-0126 Decision 二 — 对账门三格：相等放行；旧式无版本串与算出的不等是`未受理`；带本构建
// 不认识的规范化版本是「不支持」，不折成不等。
func TestReconcileDeclaredDigest(t *testing.T) {
	canonical := canonicalCreditPolicy(t, creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{}))

	if err := domain.ReconcileDeclaredDigest(canonical.Digest(), canonical); err != nil {
		t.Fatalf("equal digests: err = %v", err)
	}
	legacy := commercialValue(t, domain.NewCommercialContentDigest, "sha256:syn-SYN-CREDIT-01-v1")
	if err := domain.ReconcileDeclaredDigest(legacy, canonical); !errors.Is(err, domain.ErrDeclaredDigestMismatch) {
		t.Fatalf("legacy declared digest: err = %v, want ErrDeclaredDigestMismatch", err)
	}
	if _, carried := legacy.Canonicalization(); carried {
		t.Fatalf("sha256: prefix must not be read as a canonicalization version")
	}
	future := commercialValue(t, domain.NewCommercialContentDigest, "PCC-9:"+strings.Repeat("0", 64))
	if err := domain.ReconcileDeclaredDigest(future, canonical); !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("future canonicalization: err = %v, want ErrCanonicalizationUnsupported", err)
	}
	if version, carried := future.Canonicalization(); !carried || version != "PCC-9" {
		t.Fatalf("Canonicalization() = %q, %v; want PCC-9, true", version, carried)
	}
	notAVersion := commercialValue(t, domain.NewCommercialContentDigest, "PCC-:abc")
	if _, carried := notAVersion.Canonicalization(); carried {
		t.Fatalf("PCC- without digits must not be read as a version")
	}
}
