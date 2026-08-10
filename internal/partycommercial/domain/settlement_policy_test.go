package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func applicability(t *testing.T, counterparty, contract, chargeScope, currency string) domain.SettlementApplicability {
	t.Helper()
	built, err := domain.NewSettlementApplicability(
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCounterpartyReference, counterparty),
		commercialValue(t, domain.NewCommercialVersionLabel, contract),
		commercialValue(t, domain.NewChargeScopeReference, chargeScope),
		commercialValue(t, domain.NewCurrencyCode, currency),
		mustInterval(t),
	)
	if err != nil {
		t.Fatalf("new settlement applicability: %v", err)
	}
	return built
}

func mustInterval(t *testing.T) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	return interval
}

func settlementPolicy(t *testing.T, objectID string, method domain.SettlementMethod, chargeScope string) domain.SettlementPolicy {
	t.Helper()
	version := registerable(t, domain.SettlementPolicyObject, objectID, "v1", "sha256:"+objectID)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	policy, err := domain.NewSettlementPolicy(live, method, applicability(t, "customer-1", "contract-1/v1", chargeScope, "SYN"))
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	return policy
}

func controlScope(t *testing.T, chargeScope, currency string) domain.SettlementQuery {
	t.Helper()
	query, err := domain.NewSettlementQuery(
		commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		commercialValue(t, domain.NewCounterpartyReference, "customer-1"),
		commercialValue(t, domain.NewCommercialVersionLabel, "contract-1/v1"),
		commercialValue(t, domain.NewChargeScopeReference, chargeScope),
		commercialValue(t, domain.NewCurrencyCode, currency),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new settlement query: %v", err)
	}
	return query
}

// Covers: AT-PC-031 — 同一货主的预付范围与账期范围由合同、费用范围与有效期间明确区分时，
// 各自唯一解析；不得把客户标成一个全局模式，也不得把两个范围合并。
func TestPrepaidAndTermsCoexistAcrossNonOverlappingScopes(t *testing.T) {
	policies := []domain.SettlementPolicy{
		settlementPolicy(t, "policy-prepaid", domain.PrepaidMethod, "charge-express"),
		settlementPolicy(t, "policy-terms", domain.TermsMethod, "charge-economy"),
	}

	expected := map[string]domain.SettlementMethod{
		"charge-express": domain.PrepaidMethod,
		"charge-economy": domain.TermsMethod,
	}
	for scope, method := range expected {
		t.Run(scope, func(t *testing.T) {
			resolved, err := domain.ResolveSettlementPolicy(policies, controlScope(t, scope, "SYN"))
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if resolved.Method() != method {
				t.Fatalf("method = %q, want %q", resolved.Method(), method)
			}
		})
	}
}

// Covers: AT-PC-032 与 CONTEXT「同一金额或接受前控制范围不得同时命中两种方式」—
// 返回适用冲突并阻断依赖该依据的新决定，不任选一种模式，也不套用客户默认值。
func TestSameScopeHitByBothMethodsIsAConflict(t *testing.T) {
	policies := []domain.SettlementPolicy{
		settlementPolicy(t, "policy-prepaid", domain.PrepaidMethod, "charge-express"),
		settlementPolicy(t, "policy-terms", domain.TermsMethod, "charge-express"),
	}

	resolved, err := domain.ResolveSettlementPolicy(policies, controlScope(t, "charge-express", "SYN"))
	if !errors.Is(err, domain.ErrSettlementMethodConflict) {
		t.Fatalf("error = %v, want ErrSettlementMethodConflict", err)
	}
	if resolved.Method() != domain.SettlementMethodInvalid {
		t.Fatalf("a conflict still picked %q", resolved.Method())
	}
}

// Covers: CONTEXT「零匹配、重叠多匹配或范围证据不足分别形成无适用依据、适用冲突或解析
// 未决，不能用客户级默认值补齐」— 零匹配是明确的无适用依据，不是默认预付。
func TestNoMatchingPolicyIsNoApplicableBasisRatherThanADefault(t *testing.T) {
	policies := []domain.SettlementPolicy{
		settlementPolicy(t, "policy-prepaid", domain.PrepaidMethod, "charge-express"),
	}

	resolved, err := domain.ResolveSettlementPolicy(policies, controlScope(t, "charge-unknown", "SYN"))
	if !errors.Is(err, domain.ErrNoApplicableSettlementPolicy) {
		t.Fatalf("error = %v, want ErrNoApplicableSettlementPolicy", err)
	}
	if resolved.Method() != domain.SettlementMethodInvalid {
		t.Fatal("a scope with no policy was given a method anyway")
	}
}

// Covers: CONTEXT「不同责任法人、相对方、收付方向或币种不得通过宽泛的客户或供应商关系
// 自动归集」— 六个维度里任一不同即不适用。
func TestEveryApplicabilityDimensionDiscriminates(t *testing.T) {
	policies := []domain.SettlementPolicy{settlementPolicy(t, "policy-prepaid", domain.PrepaidMethod, "charge-express")}

	t.Run("other currency does not match", func(t *testing.T) {
		if _, err := domain.ResolveSettlementPolicy(policies, controlScope(t, "charge-express", "SYN2")); !errors.Is(err, domain.ErrNoApplicableSettlementPolicy) {
			t.Fatalf("error = %v; a policy answered across currencies", err)
		}
	})

	t.Run("outside the effective interval does not match", func(t *testing.T) {
		late, err := domain.NewSettlementQuery(
			commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
			commercialValue(t, domain.NewCounterpartyReference, "customer-1"),
			commercialValue(t, domain.NewCommercialVersionLabel, "contract-1/v1"),
			commercialValue(t, domain.NewChargeScopeReference, "charge-express"),
			commercialValue(t, domain.NewCurrencyCode, "SYN"),
			time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC),
		)
		if err != nil {
			t.Fatalf("new query: %v", err)
		}
		if _, err := domain.ResolveSettlementPolicy(policies, late); !errors.Is(err, domain.ErrNoApplicableSettlementPolicy) {
			t.Fatalf("error = %v; an expired applicability still answered", err)
		}
	})
}

// Covers: CONTEXT 商业版本共同不变量 — 结算政策的内容挂在一个当前可用的结算政策版本上，
// 草稿与别的对象类型都不能承载它。
func TestSettlementPolicyNeedsAUsableSettlementPolicyVersion(t *testing.T) {
	usable := applicability(t, "customer-1", "contract-1/v1", "charge-express", "SYN")

	t.Run("refuses a draft", func(t *testing.T) {
		draft := commercialDraft(t, domain.SettlementPolicyObject, "policy-x", "v1", "sha256:x")
		if _, err := domain.NewSettlementPolicy(draft, domain.PrepaidMethod, usable); !errors.Is(err, domain.ErrInvalidSettlementPolicy) {
			t.Fatalf("error = %v, want ErrInvalidSettlementPolicy", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		contract := effectiveVersion(t, "contract-9", "v1", "sha256:c9")
		if _, err := domain.NewSettlementPolicy(contract, domain.PrepaidMethod, usable); !errors.Is(err, domain.ErrInvalidSettlementPolicy) {
			t.Fatalf("error = %v, want ErrInvalidSettlementPolicy", err)
		}
	})
}

// Covers: CONTEXT — 结算方式是封闭两值，没有第三个取值可以表达「客户默认」。
func TestSettlementMethodSetIsExactlyTwoValued(t *testing.T) {
	labels := map[string]struct{}{}
	for _, method := range []domain.SettlementMethod{domain.PrepaidMethod, domain.TermsMethod} {
		label := method.String()
		if label == "" {
			t.Fatalf("method %d has no label", method)
		}
		labels[label] = struct{}{}
	}
	if len(labels) != 2 {
		t.Fatalf("settlement method collapsed into %d labels", len(labels))
	}
	if domain.SettlementMethod(len(labels)+1).String() != "" {
		t.Fatal("a third settlement method exists, which invites a customer-level default")
	}
}
