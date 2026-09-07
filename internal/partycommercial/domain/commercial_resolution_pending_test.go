package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: `S01-W02` 结果表要求 `RESOLUTION_PENDING` 可回显「稳定原因」与「尝试/续办关联」，
// UC-PC-002 要求未决「保存缺口并安全续办」。两条初始未决路径此前两样都不带，消费者拿到未决
// 却无从续办，而同一结果类型经提交前重解产生时却是带的（当年是单依据形态 ValidateBeforeDecision，
// 已随票 wiring-baseline-remainder/05 删去；今天那一侧是闭包形态 ValidateClosureBeforeDecision）。
func TestInitialPendingCarriesItsReasonAndContinuation(t *testing.T) {
	t.Run("anchor policy not configured", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

		key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
		key.Anchor = domain.SelectionAnchor{}

		result := domain.ResolveCommercialBasis(registry, key, nil)

		if result.Outcome() != domain.ResolutionPending {
			t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
		}
		if result.Reason() != domain.AnchorPolicyNotConfigured {
			t.Fatalf("reason = %q, want ANCHOR_POLICY_NOT_CONFIGURED", result.Reason())
		}
		if result.ContinuationReference().String() == "" {
			t.Fatal("a pending result offers no continuation, so the caller cannot safely resume it")
		}
	})

	t.Run("authority unreadable", func(t *testing.T) {
		result := domain.ResolveCommercialBasis(nil, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

		if result.Outcome() != domain.ResolutionPending {
			t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
		}
		if result.Reason() != domain.AuthorityUnreadable {
			t.Fatalf("reason = %q, want AUTHORITY_UNREADABLE", result.Reason())
		}
		if result.ContinuationReference().String() == "" {
			t.Fatal("a pending result offers no continuation, so the caller cannot safely resume it")
		}
	})
}

// Covers: 「稳定」的实际含义 — 同一输入因同一原因未决时必须给出同一引用，否则该引用无法用来
// 查询原处理尝试，安全续办也就无从谈起。
func TestPendingContinuationIsStableAcrossRepeatedResolution(t *testing.T) {
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)

	first := domain.ResolveCommercialBasis(nil, key, nil)
	second := domain.ResolveCommercialBasis(nil, key, nil)

	if first.ContinuationReference() != second.ContinuationReference() {
		t.Fatalf("continuation drifted between identical pending resolutions: %q vs %q",
			first.ContinuationReference().String(), second.ContinuationReference().String())
	}
}

// Covers: 成功解析没有原因可言，原因字段不得被顺手填上一个看起来无害的默认值。
func TestUniqueResolutionCarriesNoReason(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	if result.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
	}
	if result.Reason() != domain.ResolutionReasonNone {
		t.Fatalf("a unique resolution named a reason: %q", result.Reason())
	}
}
