package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func rulePackage(t *testing.T) domain.CommercialVersion {
	t.Helper()
	published, err := commercialDraft(t, domain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:rules-1").
		Publish(approval(t, "approval-rules-1"), domain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("publish rule package: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func asOfPolicy(t *testing.T, judgment domain.JudgmentType, semantics, policyVersion string) domain.AsOfPolicy {
	t.Helper()
	policy, err := domain.NewAsOfPolicy(
		judgment,
		commercialValue(t, domain.NewAsOfSemanticsReference, semantics),
		commercialValue(t, domain.NewAsOfPolicyVersion, policyVersion),
	)
	if err != nil {
		t.Fatalf("new as-of policy: %v", err)
	}
	return policy
}

// Covers: AT-PC-023 — 规则包选出后为网络与财务分别声明不同 `asOf`，两者独立形成，
// 不得压成一个全局时间。
func TestRulePackageDeclaresIndependentAsOfPerJudgment(t *testing.T) {
	declaration, err := domain.DeclareAsOfPolicies(rulePackage(t), []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1"),
		asOfPolicy(t, domain.PreAcceptanceFinancialControlJudgment, "semantics-financial", "asof-policy-v2"),
	})
	if err != nil {
		t.Fatalf("declare as-of policies: %v", err)
	}

	network, err := declaration.FormAsOf(domain.NetworkReachabilityJudgment, time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("form network as-of: %v", err)
	}
	financial, err := declaration.FormAsOf(domain.PreAcceptanceFinancialControlJudgment, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("form financial as-of: %v", err)
	}

	if network.At().Equal(financial.At()) {
		t.Fatal("two judgments were collapsed onto one instant")
	}
	if network.Policy().Semantics() == financial.Policy().Semantics() {
		t.Fatal("two judgments shared one as-of semantics reference")
	}
	if network.Policy().PolicyVersion().String() != "asof-policy-v1" ||
		financial.Policy().PolicyVersion().String() != "asof-policy-v2" {
		t.Fatal("formed as-of did not echo the policy version it came from")
	}
	if network.Judgment() != domain.NetworkReachabilityJudgment {
		t.Fatalf("judgment = %q, want NETWORK_REACHABILITY", network.Judgment())
	}
}

// Covers: UC-PC-002 第二阶段失败边界「未配置」— 未声明策略的判断类型必须显式失败，
// 不得沿用另一判断的策略或任何默认时点。
func TestUndeclaredJudgmentHasNoAsOfAndNoFallback(t *testing.T) {
	declaration, err := domain.DeclareAsOfPolicies(rulePackage(t), []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1"),
	})
	if err != nil {
		t.Fatalf("declare: %v", err)
	}

	if _, present := declaration.PolicyFor(domain.PreAcceptanceFinancialControlJudgment); present {
		t.Fatal("an undeclared judgment reported a policy")
	}
	if _, err := declaration.FormAsOf(domain.PreAcceptanceFinancialControlJudgment, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)); !errors.Is(err, domain.ErrAsOfPolicyNotConfigured) {
		t.Fatalf("error = %v, want ErrAsOfPolicyNotConfigured", err)
	}
}

// Covers: UC-PC-002 第二阶段失败边界「值不合法」— 消费方形成的值必须显式给出，
// 零值不得被当作「现在」。
func TestFormingAsOfRequiresAnExplicitValue(t *testing.T) {
	declaration, err := domain.DeclareAsOfPolicies(rulePackage(t), []domain.AsOfPolicy{
		asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1"),
	})
	if err != nil {
		t.Fatalf("declare: %v", err)
	}

	if _, err := declaration.FormAsOf(domain.NetworkReachabilityJudgment, time.Time{}); !errors.Is(err, domain.ErrInvalidAsOfValue) {
		t.Fatalf("error = %v, want ErrInvalidAsOfValue", err)
	}
}

// Covers: party-commercial CONTEXT 只有规则包唯一选出后才能声明 `asOf` 策略 —
// 声明必须挂在一个当前可用的接单规则包上。
func TestAsOfDeclarationRequiresAUsableRulePackage(t *testing.T) {
	policies := []domain.AsOfPolicy{asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1")}

	t.Run("refuses a draft rule package", func(t *testing.T) {
		draft := commercialDraft(t, domain.AcceptanceRulePackageObject, "rules-2", "v1", "sha256:rules-2")
		if _, err := domain.DeclareAsOfPolicies(draft, policies); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("refuses an object that is not a rule package", func(t *testing.T) {
		contract := effectiveVersion(t, "contract-1", "v1", "sha256:contract-1")
		if _, err := domain.DeclareAsOfPolicies(contract, policies); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("refuses a rule package that has ended", func(t *testing.T) {
		retired, err := rulePackage(t).Retire(commercialValue(t, domain.NewRetirementReference, "retire-rules"), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("retire: %v", err)
		}
		if _, err := domain.DeclareAsOfPolicies(retired, policies); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})

	t.Run("refuses a declaration with no policy at all", func(t *testing.T) {
		if _, err := domain.DeclareAsOfPolicies(rulePackage(t), nil); !errors.Is(err, domain.ErrAsOfPolicyNotConfigured) {
			t.Fatalf("error = %v, want ErrAsOfPolicyNotConfigured", err)
		}
	})

	t.Run("refuses two policies for one judgment", func(t *testing.T) {
		duplicated := []domain.AsOfPolicy{
			asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-network", "asof-policy-v1"),
			asOfPolicy(t, domain.NetworkReachabilityJudgment, "semantics-other", "asof-policy-v3"),
		}
		if _, err := domain.DeclareAsOfPolicies(rulePackage(t), duplicated); !errors.Is(err, domain.ErrConflictingAsOfPolicy) {
			t.Fatalf("error = %v, want ErrConflictingAsOfPolicy", err)
		}
	})
}

// Covers: party-commercial CONTEXT 规则包只装配规则引用，不得替具体委托选择判断值 —
// 语义引用是不透明的，本上下文不解释它，也不内置任何取值集合。
func TestAsOfSemanticsStayOpaqueToThisContext(t *testing.T) {
	policy := asOfPolicy(t, domain.NetworkReachabilityJudgment, "SEMANTICS-DEFINED-ELSEWHERE", "asof-policy-v1")

	if policy.Semantics().String() != "SEMANTICS-DEFINED-ELSEWHERE" {
		t.Fatalf("semantics = %q; this context rewrote a reference it does not own", policy.Semantics())
	}
	if _, err := domain.NewAsOfSemanticsReference(""); err == nil {
		t.Fatal("an empty semantics reference constructed, which would become an implicit default")
	}
}
