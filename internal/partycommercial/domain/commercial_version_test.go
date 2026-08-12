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
		TenantID:      commercialValue(t, domain.NewTenantID, "tenant-1"),
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

// Covers: `AT-PC-012`「已发布价格正文需要修改 → 创建新价格规则版本，不原地编辑」——发布后
// `Revise` 被 `ErrCommercialContentIsFixed` 拒绝；变化只能另起版本（并存见登记册用例）。
func TestPublishedCommercialVersionRefusesInPlaceRevision(t *testing.T) {
	draft := commercialDraft(t, domain.PriceRuleObject, "price-1", "v1", "sha256:content-1")

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

	published, err := revised.Publish(approval(t, "approval-1"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
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
			if _, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, publishAt, nil); !errors.Is(err, domain.ErrIncompleteCommercialPublication) {
				t.Fatalf("error = %v, want ErrIncompleteCommercialPublication", err)
			}
		})
	}

	t.Run("publication cannot precede approval", func(t *testing.T) {
		early := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		if _, err := draft.Publish(approval(t, "approval-2"), domain.ApprovalRoleConfirmed, early, nil); !errors.Is(err, domain.ErrIncompleteCommercialPublication) {
			t.Fatalf("error = %v, want ErrIncompleteCommercialPublication", err)
		}
	})
}

// Covers: `AT-PC-010`「导入成功但批准角色未确认 → 保留来源，发布保持未决」。
//
// 依据字段齐全时仍不得发布：角色未确认是另一格，不得压成 Incomplete（ADR-0035）。
func TestPublicationWaitsWhenApprovalRoleIsUnconfirmed(t *testing.T) {
	draft := commercialDraft(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:imported-source")
	publishAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	basis := approval(t, "approval-ready")

	for name, standing := range map[string]domain.ApprovalRoleStanding{
		"explicitly unconfirmed": domain.ApprovalRoleUnconfirmed,
		"unanswered zero value":  domain.ApprovalRoleStandingInvalid,
	} {
		t.Run(name, func(t *testing.T) {
			published, err := draft.Publish(basis, standing, publishAt, nil)
			if !errors.Is(err, domain.ErrApprovalRoleNotConfirmed) {
				t.Fatalf("error = %v, want ErrApprovalRoleNotConfirmed", err)
			}
			if errors.Is(err, domain.ErrIncompleteCommercialPublication) {
				t.Fatal("角色未确认被压成了依据字段不全")
			}
			if published.Status() == domain.CommercialVersionPublished {
				t.Fatal("角色未确认时仍然发布成功了")
			}
			if draft.Status() != domain.CommercialVersionDraft {
				t.Fatal("角色未确认时草稿被改写了")
			}
			if draft.ContentDigest().String() != "sha256:imported-source" {
				t.Fatal("导入来源正文在未确认发布时丢失了")
			}
		})
	}

	t.Run("confirmed role publishes", func(t *testing.T) {
		published, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, publishAt, nil)
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if published.Status() != domain.CommercialVersionPublished {
			t.Fatalf("status = %q, want PUBLISHED", published.Status())
		}
	})

	t.Run("the two refusals stay distinguishable", func(t *testing.T) {
		if errors.Is(domain.ErrApprovalRoleNotConfirmed, domain.ErrIncompleteCommercialPublication) ||
			errors.Is(domain.ErrIncompleteCommercialPublication, domain.ErrApprovalRoleNotConfirmed) {
			t.Fatal("两个哨兵互相 Is，调用方分不出该补字段还是该等角色确认")
		}
	})
}

// Covers: `AT-PC-005`「合同引用尚未发布的规则包 → 合同发布未决，不建立悬空生产引用」。
//
// 与 AT-PC-022 分清：022 是解析闭包确认；本条是发布闸门。两边都要守（ADR-0036）。
func TestPublicationWaitsWhenNamedReferenceIsUnpublished(t *testing.T) {
	spec := commercialSpec(t, domain.CustomerContractObject, "contract-1", "v1", "sha256:imported")
	spec.References = map[domain.CommercialObjectKind]domain.CommercialObjectID{
		domain.AcceptanceRulePackageObject: commercialValue(t, domain.NewCommercialObjectID, "rules-unpublished"),
	}
	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	publishAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	basis := approval(t, "approval-ready")

	for name, standing := range map[string]domain.NamedReferenceStandingLookup{
		"explicitly unpublished": func(domain.CommercialObjectKind, domain.CommercialObjectID) domain.NamedReferenceStanding {
			return domain.NamedReferenceUnpublished
		},
		"unanswered nil lookup": nil,
		"unanswered zero value": func(domain.CommercialObjectKind, domain.CommercialObjectID) domain.NamedReferenceStanding {
			return domain.NamedReferenceStandingInvalid
		},
	} {
		t.Run(name, func(t *testing.T) {
			published, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, publishAt, standing)
			if !errors.Is(err, domain.ErrNamedReferenceNotPublished) {
				t.Fatalf("error = %v, want ErrNamedReferenceNotPublished", err)
			}
			if errors.Is(err, domain.ErrIncompleteCommercialPublication) || errors.Is(err, domain.ErrApprovalRoleNotConfirmed) {
				t.Fatal("指名引用未发布被压成了另一格未决")
			}
			if published.Status() == domain.CommercialVersionPublished {
				t.Fatal("未发布引用仍建成了生产合同")
			}
			if draft.Status() != domain.CommercialVersionDraft {
				t.Fatal("发布未决时草稿被改写了")
			}
			if draft.ContentDigest().String() != "sha256:imported" {
				t.Fatal("导入来源正文在发布未决时丢失了")
			}
		})
	}

	t.Run("published named reference may publish", func(t *testing.T) {
		published, err := draft.Publish(basis, domain.ApprovalRoleConfirmed, publishAt,
			func(domain.CommercialObjectKind, domain.CommercialObjectID) domain.NamedReferenceStanding {
				return domain.NamedReferencePublished
			})
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if published.Status() != domain.CommercialVersionPublished {
			t.Fatalf("status = %q, want PUBLISHED", published.Status())
		}
	})

	t.Run("the three publication refusals stay distinguishable", func(t *testing.T) {
		refusals := []error{
			domain.ErrIncompleteCommercialPublication,
			domain.ErrApprovalRoleNotConfirmed,
			domain.ErrNamedReferenceNotPublished,
		}
		for i := 0; i < len(refusals); i++ {
			for j := i + 1; j < len(refusals); j++ {
				if errors.Is(refusals[i], refusals[j]) || errors.Is(refusals[j], refusals[i]) {
					t.Fatalf("%v 与 %v 互相 Is", refusals[i], refusals[j])
				}
			}
		}
	})
}

// Covers: party-commercial CONTEXT — 各对象保持独立身份，不合并为一份「大配置」。
// 九种类别的封闭集合由上下文声明，因此集合之外的取值不得构造出来。
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

		// 每种类别各按自己的身份发布；跨类别共用一个对象 ID 不得让它们变成同一个对象。
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
	// 不完整的依据本就不该构造得出来；发布对零值也必须拒绝得同样干脆，调用方断言的正是这一点。
	return domain.ApprovalBasis{}
}
