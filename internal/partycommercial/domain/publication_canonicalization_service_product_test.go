package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证服务产品册接进 PCC-1 的形（票 admin-write-faces/09「本册规范化判断」）：本册没有正文，规范化文档
// 只剩规范化版本与 kind 两格，每一版摘要同一个串；同键重发是重放、换壳是修订，判据没有一处为本册另开。

func canonicalServiceProduct(t *testing.T) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.ServiceProductObject})
	if err != nil {
		t.Fatalf("canonicalize service product: %v", err)
	}
	return canonical
}

func serviceProductShell(t *testing.T, scope string, references map[domain.CommercialObjectKind]domain.CommercialObjectID) domain.PublicationDraftShell {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.PublicationDraftShell{
		TenantID:   commercialValue(t, domain.NewTenantID, "tenant-1"),
		Kind:       domain.ServiceProductObject,
		ObjectID:   commercialValue(t, domain.NewCommercialObjectID, "product-1"),
		Version:    commercialValue(t, domain.NewCommercialVersionLabel, "v1"),
		Scope:      commercialValue(t, domain.NewCommercialScopeReference, scope),
		Effective:  interval,
		References: references,
	}
}

// Covers: 票 09「本册规范化判断」第 2、3 条 — 无正文册接进同一个 PCC-1 不换号；文档恰是 {canonicalization, kind}
// 两格，摘要串带版本；两次算逐字节同串。
func TestServiceProductCanonicalizesToTheTwoFieldDocument(t *testing.T) {
	first := canonicalServiceProduct(t)
	second := canonicalServiceProduct(t)

	if first.Digest() != second.Digest() {
		t.Fatalf("same (empty) body canonicalized twice: %s vs %s", first.Digest(), second.Digest())
	}
	// 版本号按 ADR-0126 Decision 一的原文写死：加册不换号，本册接进来仍是 PCC-1。
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("canonicalization = %q, digest = %s; want PCC-1", first.Canonicalization(), first.Digest())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(first.Document(), &document); err != nil {
		t.Fatalf("document is not JSON: %v", err)
	}
	if len(document) != 2 || string(document["canonicalization"]) != `"PCC-1"` || string(document["kind"]) != `"SERVICE_PRODUCT"` {
		t.Fatalf("document = %s; want exactly {canonicalization, kind}", first.Document())
	}
	if !domain.IsRegisterCanonicalized(domain.ServiceProductObject) {
		t.Fatal("IsRegisterCanonicalized(SERVICE_PRODUCT) must answer true once the register is wired")
	}
}

// Covers: 票 09「本册规范化判断」第 1 条 — 壳上的指名引用不进文档：换引用不换串。信用政策的正文冒服务产品的名
// 仍答 kind 不符（既有那一格对本册照旧成立）。
func TestServiceProductDigestIgnoresTheShellAndRefusesForeignBodies(t *testing.T) {
	bare := canonicalServiceProduct(t)
	withReference, err := domain.PreviewPublication(serviceProductShell(t, "scope-1", map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.CustomerContractObject: commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
	}), domain.PublicationContent{Kind: domain.ServiceProductObject})
	if err != nil {
		t.Fatalf("preview with a reference: %v", err)
	}
	if withReference.Digest() != bare.Digest() {
		t.Fatalf("a shell reference changed the digest: %s vs %s", withReference.Digest(), bare.Digest())
	}

	body := creditPolicyBody(t, "freight", creditAmount(t, 100), time.Time{})
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.ServiceProductObject, CreditPolicy: &body})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("credit body under service product kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}
}

// Covers: 票 09「本册规范化判断」第 2 条 — 同键重发（壳同）是重放；换范围或换引用是修订而不是重放：摘要同串，
// 分辨靠 SameSubmissionAs 比壳，本册没有另一套判据。
func TestServiceProductDraftsReplayOnSameShellAndReviseOnShellChange(t *testing.T) {
	submitter := commercialValue(t, domain.NewOperatorSubjectReference, "op-1")
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	content := domain.PublicationContent{Kind: domain.ServiceProductObject}
	submit := func(shell domain.PublicationDraftShell) domain.PublicationDraft {
		draft, err := domain.SubmitPublicationDraft(shell, content, submitter, at)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		return draft
	}

	original := submit(serviceProductShell(t, "scope-1", nil))
	again := submit(serviceProductShell(t, "scope-1", nil))
	if !original.SameContentAs(again) || !original.SameSubmissionAs(again) {
		t.Fatal("the same shell submitted twice must be the same submission")
	}
	rescoped := submit(serviceProductShell(t, "scope-2", nil))
	if !original.SameContentAs(rescoped) || original.SameSubmissionAs(rescoped) {
		t.Fatal("a rescoped shell shares the digest but is not the same submission")
	}
	referenced := submit(serviceProductShell(t, "scope-1", map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.CustomerContractObject: commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
	}))
	if !original.SameContentAs(referenced) || original.SameSubmissionAs(referenced) {
		t.Fatal("adding a shell reference shares the digest but is not the same submission")
	}
	if original.PublicationSpec().ContentDigest != original.Canonical().Digest() {
		t.Fatal("the publication spec must carry the computed digest")
	}
}

// Covers: 快照折回 — 存下来的两格文档折回本册的正文输入面，不答「正文缺席」；载体整份重建后摘要与列上一致。
func TestServiceProductSnapshotRehydratesWithoutABody(t *testing.T) {
	canonical := canonicalServiceProduct(t)
	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate service product content: %v", err)
	}
	if content.Kind != domain.ServiceProductObject || content.CreditPolicy != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}

	submitter := commercialValue(t, domain.NewOperatorSubjectReference, "op-1")
	rehydrated, err := domain.RehydratePublicationDraft(domain.RehydratePublicationDraftSpec{
		Shell:            serviceProductShell(t, "scope-1", nil),
		Canonicalization: canonical.Canonicalization(),
		Document:         canonical.Document(),
		Digest:           canonical.Digest(),
		Submitter:        submitter,
		SubmittedAt:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Status:           domain.PublicationDraftPendingApproval,
	})
	if err != nil {
		t.Fatalf("rehydrate service product draft: %v", err)
	}
	if rehydrated.Canonical().Digest() != canonical.Digest() || rehydrated.Kind() != domain.ServiceProductObject {
		t.Fatalf("rehydrated draft = %#v", rehydrated)
	}

	// 快照里塞进别册的正文：kind 是服务产品而文档带信用政策一节，规范化时按 kind 不符拒——行坏了不假装能读。
	forged := strings.Replace(string(canonical.Document()), `"kind":"SERVICE_PRODUCT"`,
		`"kind":"SERVICE_PRODUCT","creditPolicy":{"legalEntity":"l","authorityLevel":"a","chargeType":"c","limitMinor":1,"effectiveStartsAt":"2026-01-01T00:00:00Z"}`, 1)
	if _, err := domain.RehydratePublicationDraft(domain.RehydratePublicationDraftSpec{
		Shell:            serviceProductShell(t, "scope-1", nil),
		Canonicalization: canonical.Canonicalization(),
		Document:         []byte(forged),
		Digest:           canonical.Digest(),
		Submitter:        submitter,
		SubmittedAt:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Status:           domain.PublicationDraftPendingApproval,
	}); !errors.Is(err, domain.ErrInvalidRehydratedPublicationDraft) {
		t.Fatalf("forged snapshot: err = %v, want ErrInvalidRehydratedPublicationDraft", err)
	}
}
