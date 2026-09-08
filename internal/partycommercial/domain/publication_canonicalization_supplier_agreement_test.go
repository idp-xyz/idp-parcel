package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证供应商协议册接进服务端规范化（票 admin-write-faces/11；ADR-0126 Decision 一「加册不换号」）：
// 同一份正文逐字节同摘要、文档键名镜像受控批文的 supplierAgreementBody、三格拒绝各归其格、文档能折回正文。

func supplierAgreementBody(t *testing.T, purchasePlan string, endsAt time.Time) domain.SupplierAgreementBody {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.SupplierAgreementBody{
		Supplier:     commercialValue(t, domain.NewPartyID, "supplier-1"),
		LegalEntity:  commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:        commercialValue(t, domain.NewCommercialScopeReference, "scope-buy-1"),
		PurchasePlan: commercialValue(t, domain.NewPricingPlanReference, purchasePlan),
		Effective:    interval,
	}
}

func canonicalSupplierAgreement(t *testing.T, body domain.SupplierAgreementBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:              domain.SupplierAgreementObject,
		SupplierAgreement: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一 — 供应商协议接进 PCC-1 不换号；同一正文两次算逐字节同串；文档键名镜像批文
// supplierAgreementBodyDocument（supplier / legalEntity / scope / purchasePlan / effectiveStartsAt / effectiveEndsAt），
// 没有方向键，区间无上界时 effectiveEndsAt 缺席。
func TestSupplierAgreementCanonicalizesIntoTheSameVersionMirroringTheBatchDocument(t *testing.T) {
	body := supplierAgreementBody(t, "plan-buy-1", time.Time{})
	first := canonicalSupplierAgreement(t, body)
	second := canonicalSupplierAgreement(t, body)

	if first.Digest() != second.Digest() {
		t.Fatalf("same body canonicalized twice: %s vs %s", first.Digest(), second.Digest())
	}
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("canonicalization = %q, digest = %q; want PCC-1", first.Canonicalization(), first.Digest())
	}
	// 文档字节整份钉死：键名与顺序就是规范化的全部，改一处即另一个版本号（ADR-0014）。
	wantDocument := `{"canonicalization":"PCC-1","kind":"SUPPLIER_AGREEMENT",` +
		`"supplierAgreement":{"supplier":"supplier-1","legalEntity":"legal-1","scope":"scope-buy-1",` +
		`"purchasePlan":"plan-buy-1","effectiveStartsAt":"2026-01-03T00:00:00Z"}}`
	if got := string(first.Document()); got != wantDocument {
		t.Fatalf("document = %s\nwant       %s", got, wantDocument)
	}
	if strings.Contains(string(first.Document()), "direction") {
		t.Fatalf("a supplier agreement document must not carry a direction key: %s", first.Document())
	}

	bounded := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-1", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))
	if !strings.Contains(string(bounded.Document()), `"effectiveEndsAt":"2026-06-30T16:00:00Z"`) {
		t.Fatalf("bounded interval did not write its end: %s", bounded.Document())
	}
	if bounded.Digest() == first.Digest() {
		t.Fatal("bounded and open-ended intervals must not share a digest")
	}
}

// Covers: ADR-0126 Decision 一 — 正文任一格不同即另一个串（换采购方案）；同一时刻不同时区写法不产生第二个串。
func TestSupplierAgreementDigestDistinguishesContentButNotSpelling(t *testing.T) {
	planA := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-1", time.Time{}))
	planB := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-2", time.Time{}))
	if planA.Digest() == planB.Digest() {
		t.Fatal("two purchase plans must not share a digest")
	}

	shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
	inUTC := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-1", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))
	inShanghai := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-1", time.Date(2026, 7, 1, 0, 0, 0, 0, shanghai)))
	if inUTC.Digest() != inShanghai.Digest() {
		t.Fatalf("same instant in two zones produced two digests: %s vs %s", inUTC.Digest(), inShanghai.Digest())
	}
}

// Covers: ADR-0126 Decision 一 — 三格分开：正文缺席、正文与类别不符（两个方向）、零值正文；本册从此算「已接」。
func TestSupplierAgreementCanonicalizationRefusals(t *testing.T) {
	body := supplierAgreementBody(t, "plan-buy-1", time.Time{})
	credit := creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{})

	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.SupplierAgreementObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("supplier agreement without body: err = %v, want ErrPublicationContentAbsent", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:              domain.CreditPolicyObject,
		SupplierAgreement: &body,
	})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("supplier body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:         domain.SupplierAgreementObject,
		CreditPolicy: &credit,
	})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("credit body under supplier kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:              domain.SupplierAgreementObject,
		SupplierAgreement: &domain.SupplierAgreementBody{},
	})
	if !errors.Is(err, domain.ErrInvalidSupplierAgreement) {
		t.Fatalf("zero body: err = %v, want ErrInvalidSupplierAgreement", err)
	}
	if !domain.IsRegisterCanonicalized(domain.SupplierAgreementObject) {
		t.Fatal("IsRegisterCanonicalized: supplier agreement is canonicalized by this build")
	}
}

// Covers: ADR-0126 Decision 三 — 载体快照就是规范化文档：供应商协议文档折回正文、再算同摘要；只有壳的文档折不回。
func TestSupplierAgreementDocumentRoundTripsTheContent(t *testing.T) {
	canonical := canonicalSupplierAgreement(t, supplierAgreementBody(t, "plan-buy-1", time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)))

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate content: %v", err)
	}
	if content.Kind != domain.SupplierAgreementObject || content.SupplierAgreement == nil || content.CreditPolicy != nil {
		t.Fatalf("content = %#v", content)
	}
	if content.SupplierAgreement.PurchasePlan.String() != "plan-buy-1" || content.SupplierAgreement.Supplier.String() != "supplier-1" {
		t.Fatalf("body = %#v", *content.SupplierAgreement)
	}
	if endsAt, bounded := content.SupplierAgreement.Effective.EndsAt(); !bounded || !endsAt.Equal(time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("interval end did not travel: %v, %v", endsAt, bounded)
	}
	again, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("re-canonicalize: %v", err)
	}
	if again.Digest() != canonical.Digest() {
		t.Fatalf("round trip changed the digest: %s vs %s", again.Digest(), canonical.Digest())
	}

	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(`{"canonicalization":"PCC-1","kind":"SUPPLIER_AGREEMENT"}`)); !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("a document without its body: err = %v, want ErrPublicationContentAbsent", err)
	}
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(),
		[]byte(`{"canonicalization":"PCC-1","kind":"SUPPLIER_AGREEMENT","supplierAgreement":{"supplier":"","legalEntity":"legal-1","scope":"s","purchasePlan":"p","effectiveStartsAt":"2026-01-03T00:00:00Z"}}`)); err == nil {
		t.Fatal("a body that fails its constructor gate must not rehydrate")
	}
}
