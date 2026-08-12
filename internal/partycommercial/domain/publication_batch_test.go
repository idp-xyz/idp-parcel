package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: `AT-PC-011`「一个批次中产品合法、合同冲突 → 产品可以发布，合同冲突；批次不全量回滚」。
//
// 权威在逐对象结果（ADR-0037）：合同 CONFLICT 不得把已 CREATED 的产品撤出登记册。
func TestPublicationBatchKeepsLegalProductWhenContractConflicts(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	prior := registerable(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-original")
	if _, err := registry.Register(prior); err != nil {
		t.Fatalf("seed prior contract: %v", err)
	}
	before := registry.Count()

	publishedAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	productDraft := commercialDraft(t, domain.ServiceProductObject, "product-1", "v1", "sha256:product-ok")
	conflictDraft := commercialDraft(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-CHANGED")

	results := registry.PublishBatch([]domain.PublicationBatchItem{
		{
			Draft:        productDraft,
			Basis:        approval(t, "approval-product-1-v1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			PublishedAt:  publishedAt,
		},
		{
			Draft:        conflictDraft,
			Basis:        approval(t, "approval-contract-1-v1-changed"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			PublishedAt:  publishedAt,
		},
	}, nil)

	if len(results) != 2 {
		t.Fatalf("batch results = %d, want 2 independent outcomes", len(results))
	}

	productResult := results[0]
	if productResult.Err() != nil {
		t.Fatalf("legal product failed: %v", productResult.Err())
	}
	if productResult.Registration() != domain.RegistrationCreated {
		t.Fatalf("product registration = %q, want CREATED", productResult.Registration())
	}
	publishedProduct, ok := productResult.Published()
	if !ok || publishedProduct.Status() != domain.CommercialVersionPublished {
		t.Fatal("product was not published")
	}

	contractResult := results[1]
	if !errors.Is(contractResult.Err(), domain.ErrCommercialVersionConflict) {
		t.Fatalf("contract error = %v, want ErrCommercialVersionConflict", contractResult.Err())
	}
	if contractResult.Registration() != domain.RegistrationConflict {
		t.Fatalf("contract registration = %q, want CONFLICT", contractResult.Registration())
	}

	if got := registry.Count(); got != before+1 {
		t.Fatalf("registry holds %d versions, want %d (prior + product; conflict must not roll back)", got, before+1)
	}
	storedProduct, found := registry.Lookup(productDraft.Tenant(), productDraft.Kind(), productDraft.ObjectID(), productDraft.Version())
	if !found || storedProduct.ContentDigest().String() != "sha256:product-ok" {
		t.Fatal("合同冲突把已合法产品从登记册撤走了（全量回滚）")
	}
	storedContract, found := registry.Lookup(prior.Tenant(), prior.Kind(), prior.ObjectID(), prior.Version())
	if !found || storedContract.ContentDigest() != prior.ContentDigest() {
		t.Fatal("冲突尝试覆盖了既有合同正文")
	}
}
