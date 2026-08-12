package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func effectiveServiceProduct(t *testing.T) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, domain.ServiceProductObject, "product-pr", "v1", "sha256:product-pr").
		Publish(approval(t, "approval-product-pr"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish product: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

// Covers: CONTEXT「接单规则包必须明确哪些硬规则通过后可自动接受、哪些条件确实需要人工业务
// 复核」——适用校验组与人工复核指令是规则包正文声明，缺件即未配置，本上下文不代拟默认
// （ADR-0042）。
func TestRulePackageDeclaresItsAcceptanceContent(t *testing.T) {
	content, err := domain.DeclareAcceptanceRuleContent(
		rulePackage(t),
		[]domain.AcceptanceCheckGroupType{
			domain.CustomerRelationshipCheckGroup,
			domain.NetworkReachabilityCheckGroup,
		},
		domain.ManualReviewRequired,
	)
	if err != nil {
		t.Fatalf("declare acceptance rule content: %v", err)
	}
	if got := content.ApplicableGroups(); len(got) != 2 {
		t.Fatalf("applicable groups = %d, want 2", len(got))
	}
	if !content.Applies(domain.NetworkReachabilityCheckGroup) {
		t.Fatal("声明过的校验组查不回来")
	}
	if content.Applies(domain.RequiredDocumentCheckGroup) {
		t.Fatal("没声明的校验组被读成适用")
	}
	if content.ManualReview() != domain.ManualReviewRequired {
		t.Fatalf("manual review = %q, want REQUIRED", content.ManualReview())
	}
	if content.RulePackage().Kind() != domain.AcceptanceRulePackageObject {
		t.Fatal("内容声明没有钉住它所属的规则包")
	}

	t.Run("a draft rule package cannot carry the declaration", func(t *testing.T) {
		draft := commercialDraft(t, domain.AcceptanceRulePackageObject, "rules-draft", "v1", "sha256:rules-draft")
		if _, err := domain.DeclareAcceptanceRuleContent(
			draft,
			[]domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup},
			domain.ManualReviewNotRequired,
		); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("a contract is not a rule package", func(t *testing.T) {
		contract := effectiveVersionInScope(t, domain.CustomerContractObject, "contract-rc", "v1", "sha256:contract-rc", "scope-rc")
		if _, err := domain.DeclareAcceptanceRuleContent(
			contract,
			[]domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup},
			domain.ManualReviewNotRequired,
		); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("an empty group set is not configured", func(t *testing.T) {
		if _, err := domain.DeclareAcceptanceRuleContent(
			rulePackage(t), nil, domain.ManualReviewNotRequired,
		); !errors.Is(err, domain.ErrAcceptanceContentNotConfigured) {
			t.Fatalf("error = %v, want ErrAcceptanceContentNotConfigured", err)
		}
	})

	t.Run("an undeclared manual review directive is not configured", func(t *testing.T) {
		if _, err := domain.DeclareAcceptanceRuleContent(
			rulePackage(t),
			[]domain.AcceptanceCheckGroupType{domain.CustomerRelationshipCheckGroup},
			domain.ManualReviewUndeclared,
		); !errors.Is(err, domain.ErrAcceptanceContentNotConfigured) {
			t.Fatalf("error = %v, want ErrAcceptanceContentNotConfigured", err)
		}
	})

	t.Run("a duplicate group is a conflict, not silently deduplicated", func(t *testing.T) {
		_, err := domain.DeclareAcceptanceRuleContent(
			rulePackage(t),
			[]domain.AcceptanceCheckGroupType{
				domain.CustomerRelationshipCheckGroup,
				domain.CustomerRelationshipCheckGroup,
			},
			domain.ManualReviewNotRequired,
		)
		if !errors.Is(err, domain.ErrConflictingCheckGroup) {
			t.Fatalf("error = %v, want ErrConflictingCheckGroup", err)
		}
		if errors.Is(err, domain.ErrAcceptanceContentNotConfigured) {
			t.Fatal("重复组被压成了未配置")
		}
	})

	t.Run("an invalid group value is not configured", func(t *testing.T) {
		if _, err := domain.DeclareAcceptanceRuleContent(
			rulePackage(t),
			[]domain.AcceptanceCheckGroupType{domain.AcceptanceCheckGroupTypeInvalid},
			domain.ManualReviewNotRequired,
		); !errors.Is(err, domain.ErrAcceptanceContentNotConfigured) {
			t.Fatalf("error = %v, want ErrAcceptanceContentNotConfigured", err)
		}
	})
}

// Covers: UC-PS-001「只有服务产品明确允许待路由并保留该商业依据时」（`AT-PS-007` 的 PC 半边）
// ——许可归服务产品且必须携带依据引用；零值是未许可（ADR-0042）。
func TestPendingRoutingPermissionBelongsToTheServiceProductAndCarriesItsBasis(t *testing.T) {
	product := effectiveServiceProduct(t)
	basis := commercialValue(t, domain.NewPendingRoutingBasisReference, "pending-routing-basis-1")

	permission, err := domain.DeclarePendingRoutingPermission(product, basis)
	if err != nil {
		t.Fatalf("declare pending routing permission: %v", err)
	}
	if !permission.Allowed() {
		t.Fatal("已声明的许可读成未许可")
	}
	if permission.Basis() != basis {
		t.Fatal("许可丢了它的依据引用——消费方就没有东西可保存")
	}
	if permission.Product().Kind() != domain.ServiceProductObject {
		t.Fatal("许可没有钉住它所属的服务产品")
	}

	t.Run("the zero value is not allowed", func(t *testing.T) {
		var zero domain.PendingRoutingPermission
		if zero.Allowed() {
			t.Fatal("零值许可被读成允许——未配置成了默认放行")
		}
	})

	t.Run("a permission without a basis cannot be declared", func(t *testing.T) {
		if _, err := domain.DeclarePendingRoutingPermission(product, domain.PendingRoutingBasisReference{}); !errors.Is(err, domain.ErrInvalidPendingRoutingPermission) {
			t.Fatalf("error = %v, want ErrInvalidPendingRoutingPermission", err)
		}
	})

	t.Run("a rule package cannot grant pending routing", func(t *testing.T) {
		if _, err := domain.DeclarePendingRoutingPermission(rulePackage(t), basis); !errors.Is(err, domain.ErrInvalidPendingRoutingPermission) {
			t.Fatalf("error = %v, want ErrInvalidPendingRoutingPermission", err)
		}
	})

	t.Run("a published but not yet effective product cannot grant it", func(t *testing.T) {
		published, err := commercialDraft(t, domain.ServiceProductObject, "product-pub", "v1", "sha256:product-pub").
			Publish(approval(t, "approval-product-pub"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
		if err != nil {
			t.Fatalf("publish product: %v", err)
		}
		if _, err := domain.DeclarePendingRoutingPermission(published, basis); !errors.Is(err, domain.ErrInvalidPendingRoutingPermission) {
			t.Fatalf("error = %v, want ErrInvalidPendingRoutingPermission", err)
		}
	})
}
