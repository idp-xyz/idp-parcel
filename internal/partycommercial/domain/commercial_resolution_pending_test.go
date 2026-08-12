package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: `S01-W02` 结果表要求 `RESOLUTION_PENDING` 可回显「稳定原因」与「尝试/续办关联」，
// UC-PC-002 要求未决「保存缺口并安全续办」。两条初始未决路径此前两样都不带，消费者拿到未决
// 却无从续办，而同一结果类型经 ValidateBeforeDecision 产生时却是带的。
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

// Covers: 提交前重解遇到权威不可读时同样是未决，它此前有续办引用但没有可回显的原因。两条
// 未决路径必须给出同一套依据，否则消费者要按结果的来路分别处理。
func TestPendingFromPreDecisionValidationAlsoNamesItsReason(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	prior := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if prior.Outcome() != domain.UniquelyResolved {
		t.Fatalf("fixture did not resolve uniquely: %q", prior.Outcome())
	}

	stalled := domain.ValidateBeforeDecision(nil, prior, nil)

	if stalled.Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", stalled.Outcome())
	}
	if stalled.Reason() != domain.AuthorityUnreadable {
		t.Fatalf("reason = %q, want AUTHORITY_UNREADABLE", stalled.Reason())
	}
	if stalled.ContinuationReference().String() == "" {
		t.Fatal("a pending revalidation offers no continuation")
	}
}

// Covers: STALE 同样要求可回显稳定原因；此前原因只进了续办引用的摘要，读不出来。
func TestStaleNamesTheCauseThatOverturnedIt(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	prior := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")
	stale := domain.ValidateBeforeDecision(registry, prior, nil)

	if stale.Outcome() != domain.ResolutionStale {
		t.Fatalf("outcome = %q, want STALE", stale.Outcome())
	}
	if stale.Reason() != domain.CurrentResolutionChanged {
		t.Fatalf("reason = %q, want CURRENT_RESOLUTION_CHANGED", stale.Reason())
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
