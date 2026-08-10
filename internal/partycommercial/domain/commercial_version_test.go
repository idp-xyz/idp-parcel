package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func commercialSpec(t *testing.T, kind domain.CommercialObjectKind, objectID, version, digest string) domain.CommercialVersionSpec {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.CommercialVersionSpec{
		Kind:          kind,
		ObjectID:      commercialValue(t, domain.NewCommercialObjectID, objectID),
		Version:       commercialValue(t, domain.NewCommercialVersionLabel, version),
		Scope:         commercialValue(t, domain.NewCommercialScopeReference, "scope-"+objectID),
		ContentDigest: commercialValue(t, domain.NewCommercialContentDigest, digest),
		Effective:     interval,
	}
}

func commercialDraft(t *testing.T, kind domain.CommercialObjectKind, objectID, version, digest string) domain.CommercialVersion {
	t.Helper()
	draft, err := domain.NewCommercialDraft(commercialSpec(t, kind, objectID, version, digest))
	if err != nil {
		t.Fatalf("new commercial draft: %v", err)
	}
	return draft
}

func approval(t *testing.T, reference string) domain.ApprovalBasis {
	t.Helper()
	basis, err := domain.NewApprovalBasis(
		commercialValue(t, domain.NewApprovalReference, reference),
		commercialValue(t, domain.NewCommercialSourceReference, "source-"+reference),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	return basis
}

// Covers: party-commercial CONTEXT 商业版本共同不变量 — 草稿可修订，发布后正文
// 不可覆盖；变化形成新版本。
func TestPublishedCommercialVersionRefusesInPlaceRevision(t *testing.T) {
	draft := commercialDraft(t, domain.ServiceProductObject, "product-1", "v1", "sha256:content-1")

	revised, err := draft.Revise(commercialValue(t, domain.NewCommercialContentDigest, "sha256:content-2"))
	if err != nil {
		t.Fatalf("revise draft: %v", err)
	}
	if revised.ContentDigest().String() != "sha256:content-2" {
		t.Fatalf("draft revision did not take: %q", revised.ContentDigest())
	}
	if draft.ContentDigest().String() != "sha256:content-1" {
		t.Fatal("revising the draft mutated the original value")
	}

	published, err := revised.Publish(approval(t, "approval-1"), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.Status() != domain.CommercialVersionPublished {
		t.Fatalf("status = %q, want PUBLISHED", published.Status())
	}

	if _, err := published.Revise(commercialValue(t, domain.NewCommercialContentDigest, "sha256:content-3")); !errors.Is(err, domain.ErrCommercialContentIsFixed) {
		t.Fatalf("error = %v, want ErrCommercialContentIsFixed", err)
	}
	if published.ContentDigest().String() != "sha256:content-2" {
		t.Fatal("a refused revision still changed the published content")
	}
}

// Covers: party-commercial CONTEXT 草稿 → 已发布 — 批准与来源是发布的完备性
// 条件，不是独立状态；缺任一项都不能发布。
func TestCommercialPublicationRequiresCompleteApprovalBasis(t *testing.T) {
	draft := commercialDraft(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-1")
	publishAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	incomplete := map[string]domain.ApprovalBasis{
		"no approval reference": basisWithout(t, "approval"),
		"no source reference":   basisWithout(t, "source"),
		"no approval time":      basisWithout(t, "time"),
	}
	for name, basis := range incomplete {
		t.Run(name, func(t *testing.T) {
			if _, err := draft.Publish(basis, publishAt); !errors.Is(err, domain.ErrIncompleteCommercialPublication) {
				t.Fatalf("error = %v, want ErrIncompleteCommercialPublication", err)
			}
		})
	}

	t.Run("publication cannot precede approval", func(t *testing.T) {
		early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if _, err := draft.Publish(approval(t, "approval-2"), early); !errors.Is(err, domain.ErrIncompleteCommercialPublication) {
			t.Fatalf("error = %v, want ErrIncompleteCommercialPublication", err)
		}
	})
}

// Covers: party-commercial CONTEXT — 各对象保持独立身份，不合并为一份「大配置」。
// The closed set of nine kinds is declared by the context, so a value outside it
// must not construct.
func TestCommercialObjectKindsStayIndependentAndClosed(t *testing.T) {
	kinds := []domain.CommercialObjectKind{
		domain.ServiceProductObject,
		domain.CustomerContractObject,
		domain.SupplierAgreementObject,
		domain.AcceptanceRulePackageObject,
		domain.PreAcceptanceFinancialControlPolicyObject,
		domain.PriceRuleObject,
		domain.SettlementPolicyObject,
		domain.CreditPolicyObject,
		domain.AuthorizationRuleObject,
	}

	seen := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		label := kind.String()
		if label == "" {
			t.Fatalf("kind %d has no label", kind)
		}
		if _, exists := seen[label]; exists {
			t.Fatalf("two kinds share the label %q", label)
		}
		seen[label] = struct{}{}

		// Each kind publishes on its own identity; sharing an object ID across
		// kinds must not make them the same object.
		draft := commercialDraft(t, kind, "shared-id", "v1", "sha256:"+label)
		if draft.Kind() != kind {
			t.Fatalf("draft lost its kind: %q", draft.Kind())
		}
	}

	outside := domain.CommercialObjectKind(len(kinds) + 1)
	if _, err := domain.NewCommercialDraft(commercialSpec(t, outside, "product-x", "v1", "sha256:x")); !errors.Is(err, domain.ErrInvalidCommercialVersion) {
		t.Fatalf("a kind outside the closed set constructed: err = %v", err)
	}
}

func basisWithout(t *testing.T, missing string) domain.ApprovalBasis {
	t.Helper()
	reference := commercialValue(t, domain.NewApprovalReference, "approval-x")
	source := commercialValue(t, domain.NewCommercialSourceReference, "source-x")
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	switch missing {
	case "approval":
		reference = domain.ApprovalReference{}
	case "source":
		source = domain.CommercialSourceReference{}
	case "time":
		approvedAt = time.Time{}
	}

	basis, err := domain.NewApprovalBasis(reference, source, approvedAt)
	if err == nil {
		return basis
	}
	// An incomplete basis is expected not to construct; publication must reject
	// the zero value just as firmly, which is what the caller asserts.
	return domain.ApprovalBasis{}
}
