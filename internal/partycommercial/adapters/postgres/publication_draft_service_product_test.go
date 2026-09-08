package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 证无正文的册（服务产品，票 admin-write-faces/09）也能在 0028 的载体表上往返：两格文档
// 落进 content_document、摘要过 CHECK 的规范化前缀、读回经重建门不答「正文缺席」；同键换引用是修订不是重放。

func serviceProductShellIn(t *testing.T, tenant, scope, objectID, version string, references map[domain.CommercialObjectKind]domain.CommercialObjectID) domain.PublicationDraftShell {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(effectiveAtRow, effectiveAtRow.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	return domain.PublicationDraftShell{
		TenantID:   pcTenant(t, tenant),
		Kind:       domain.ServiceProductObject,
		ObjectID:   pcValue(t, domain.NewCommercialObjectID, objectID),
		Version:    pcValue(t, domain.NewCommercialVersionLabel, version),
		Scope:      pcValue(t, domain.NewCommercialScopeReference, scope),
		Effective:  interval,
		References: references,
	}
}

func pendingServiceProductDraft(t *testing.T, shell domain.PublicationDraftShell, submitter string) domain.PublicationDraft {
	t.Helper()
	draft, err := domain.SubmitPublicationDraft(shell, domain.PublicationContent{Kind: domain.ServiceProductObject},
		pcValue(t, domain.NewOperatorSubjectReference, submitter), draftSubmittedAt)
	if err != nil {
		t.Fatalf("录入服务产品载体：%v", err)
	}
	return draft
}

// Covers: 票 09「本册规范化判断」— 无正文载体往返：读回的正文面没有任何一册的正文、摘要与录入时相等；同键换指名
// 引用是修订（行被替换、引用落进行），同壳再录是重放。
func TestAServiceProductDraftRoundTripsWithoutABody(t *testing.T) {
	drafts, _, transactor, _ := newDraftRegistry(t)
	ctx := t.Context()
	shell := serviceProductShellIn(t, "tenant-1", "scope-1", "product-1", "v1", nil)
	submitted := pendingServiceProductDraft(t, shell, "op-submitter")

	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, submitted); outcome != ports.PublicationDraftSaved {
		t.Fatalf("submit outcome = %s, want SAVED", outcome)
	}
	stored, found := loadDraft(t, ctx, drafts, "tenant-1", shell)
	if !found {
		t.Fatal("录入的服务产品载体读不回来")
	}
	if !stored.SameSubmissionAs(submitted) || stored.Kind() != domain.ServiceProductObject || stored.Content().CreditPolicy != nil {
		t.Fatalf("读回的载体不是录入的那份：%#v", stored)
	}
	if stored.Canonical().Digest() != submitted.Canonical().Digest() || stored.Canonical().Canonicalization() != "PCC-1" {
		t.Fatalf("摘要 %s（%s）≠ %s", stored.Canonical().Digest(), stored.Canonical().Canonicalization(), submitted.Canonical().Digest())
	}

	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, pendingServiceProductDraft(t, shell, "op-other")); outcome != ports.PublicationDraftReplayed {
		t.Fatalf("same shell again = %s, want REPLAYED", outcome)
	}
	referenced := pendingServiceProductDraft(t, serviceProductShellIn(t, "tenant-1", "scope-1", "product-1", "v1",
		map[domain.CommercialObjectKind]domain.CommercialObjectID{
			domain.CustomerContractObject: pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		}), "op-reviser")
	if outcome := mustSubmitDraft(t, transactor, ctx, drafts, referenced); outcome != ports.PublicationDraftRevised {
		t.Fatalf("adding a reference = %s, want REVISED（换壳是修订）", outcome)
	}
	stored, _ = loadDraft(t, ctx, drafts, "tenant-1", shell)
	if reference, ok := stored.PublicationSpec().References[domain.CustomerContractObject]; !ok || reference.String() != "contract-1" {
		t.Fatalf("修订后的引用没落进行：%#v", stored.PublicationSpec().References)
	}
	if stored.Canonical().Digest() != submitted.Canonical().Digest() {
		t.Fatalf("换引用改了摘要：%s ≠ %s", stored.Canonical().Digest(), submitted.Canonical().Digest())
	}
}
