package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: party-commercial CONTEXT 第一阶段返回「解析标识、选择锚点、采用版本、有效区间
// 和当前修订标识」— 修订标识是返回契约的一部分，不是可选附加。
func TestResolutionCarriesTheAuthorityViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	revision, present := result.ViewRevision()
	if !present || revision.String() == "" {
		t.Fatal("a resolution carries no authority view revision")
	}
	if registry.ViewRevision(commercialValue(t, domain.NewCommercialScopeReference, "scope-a")) != revision {
		t.Fatal("the resolution echoed a revision the registry does not report")
	}
}

// Covers: S01-W02「唯一解析后新增同范围候选时，旧结果立即成为 STALE」以及「提交前校验
// 不能只检查已采用对象自身」— 新候选不改变已采用对象，只改变范围视图。
func TestNewCandidateInTheSameScopeStalesAPriorUniqueResult(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	adopted := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)

	prior := domain.ResolveCommercialBasis(registry, key, nil)
	if prior.Outcome() != domain.UniquelyResolved {
		t.Fatalf("prior outcome = %q, want UNIQUELY_RESOLVED", prior.Outcome())
	}

	// 被采纳的对象本身没有变动；只是范围里多了一个竞争者。
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")
	stored, found := registry.Lookup(adopted.Kind(), adopted.ObjectID(), adopted.Version())
	if !found || stored.ContentDigest() != adopted.ContentDigest() {
		t.Fatal("the adopted object changed, which would make this test prove nothing")
	}

	revalidated := domain.ValidateBeforeDecision(registry, prior, nil)
	if revalidated.Outcome() != domain.ResolutionStale {
		t.Fatalf("outcome = %q, want STALE", revalidated.Outcome())
	}
	if _, present := revalidated.AdoptedVersion(); present {
		t.Fatal("a stale result still names an adopted version")
	}
	if revalidated.ContinuationReference().String() == "" {
		t.Fatal("a stale result offers no continuation reference")
	}

	current := domain.ResolveCommercialBasis(registry, key, nil)
	if current.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("re-resolution outcome = %q, want APPLICABILITY_CONFLICT", current.Outcome())
	}
}

// Covers: AT-PC-024 — 相同输入与相同权威视图返回原结果；提交前校验放行，不重新编号。
func TestUnchangedAuthorityViewKeepsThePriorResolutionUsable(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	prior := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	revalidated := domain.ValidateBeforeDecision(registry, prior, nil)
	if revalidated.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want the prior result to stand", revalidated.Outcome())
	}
	if revalidated.ResolutionID() != prior.ResolutionID() {
		t.Fatal("re-validating an unchanged view renumbered the resolution")
	}
}

// Covers: AT-PC-026 — 原解析失效后重解形成新的解析标识，不复用旧编号。
func TestReResolutionAfterAChangeYieldsANewResolutionIdentity(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	live := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	first := domain.ResolveCommercialBasis(registry, key, nil)

	retired, err := live.Retire(commercialValue(t, domain.NewRetirementReference, "retire-1"), anchorAt.AddDate(0, -1, 0))
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	replacement := domain.NewCommercialRegistry()
	if _, err := replacement.Register(retired); err != nil {
		t.Fatalf("register retired: %v", err)
	}
	effectiveIn(t, replacement, domain.CustomerContractObject, "contract-1", "v2", "sha256:c1-v2", "scope-a")

	if domain.ValidateBeforeDecision(replacement, first, nil).Outcome() != domain.ResolutionStale {
		t.Fatal("a replaced basis left the prior resolution usable")
	}
	again := domain.ResolveCommercialBasis(replacement, key, nil)
	if again.Outcome() != domain.UniquelyResolved {
		t.Fatalf("re-resolution outcome = %q, want UNIQUELY_RESOLVED", again.Outcome())
	}
	if again.ResolutionID() == first.ResolutionID() {
		t.Fatal("re-resolution reused the superseded resolution identity")
	}
}

// Covers: party-commercial CONTEXT 范围级权威视图 — 另一范围的变化不得使本范围的解析
// 失效，否则任何客户的商业变更都会波及所有人。
func TestChangesInAnotherScopeDoNotStaleThisOne(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	prior := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	effectiveIn(t, registry, domain.CustomerContractObject, "contract-9", "v1", "sha256:c9", "scope-b")

	if got := domain.ValidateBeforeDecision(registry, prior, nil).Outcome(); got != domain.UniquelyResolved {
		t.Fatalf("outcome = %q; a neighbouring scope invalidated this resolution", got)
	}
}

// Covers: UC-PC-002 — 重解不可用时保持未决。无法证明原结果是否仍相容，既不能放行也不能
// 断言失效。
func TestUnavailableAuthorityLeavesRevalidationPendingRatherThanStale(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	prior := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)

	if got := domain.ValidateBeforeDecision(nil, prior, nil).Outcome(); got != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", got)
	}
}

// Covers: party-commercial CONTEXT 当前修订标识是单调修订引用 — 只有未成功的解析没有
// 可校验的原结果，提交前校验对它无话可说。
func TestOnlyAUniqueResolutionCanBeRevalidated(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	notResolved := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if notResolved.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("fixture outcome = %q, want NO_APPLICABLE_BASIS", notResolved.Outcome())
	}

	if got := domain.ValidateBeforeDecision(registry, notResolved, nil).Outcome(); got != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q; re-validating a non-result should return it unchanged", got)
	}
}
