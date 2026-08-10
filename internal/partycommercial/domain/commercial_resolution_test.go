package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var anchorAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func effectiveIn(t *testing.T, registry *domain.CommercialRegistry, kind domain.CommercialObjectKind, objectID, version, digest, scope string) domain.CommercialVersion {
	t.Helper()
	spec := commercialSpec(t, kind, objectID, version, digest)
	spec.Scope = commercialValue(t, domain.NewCommercialScopeReference, scope)

	draft, err := domain.NewCommercialDraft(spec)
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	published, err := draft.Publish(approval(t, "approval-"+objectID+"-"+version), time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

func resolutionKey(t *testing.T, scope string, basis domain.CommercialObjectKind) domain.ResolutionKey {
	t.Helper()
	anchor, err := domain.NewSelectionAnchor(anchorAt, commercialValue(t, domain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	return domain.ResolutionKey{
		TenantID:             commercialValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:    commercialValue(t, domain.NewCustomerAccountID, "customer-1"),
		LegalEntityCandidate: commercialValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:                commercialValue(t, domain.NewCommercialScopeReference, scope),
		RequiredBasis:        basis,
		Anchor:               anchor,
	}
}

// Covers: AT-PC-017 — 唯一适用时返回完整解析、采用版本、有效区间与选择锚点。
func TestUniqueCandidateResolvesWithItsAdoptedVersion(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	adopted := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject))

	if result.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
	}
	version, present := result.AdoptedVersion()
	if !present || version.ObjectID() != adopted.ObjectID() || version.Version() != adopted.Version() {
		t.Fatal("result does not carry the adopted version")
	}
	if result.ResolutionID().String() == "" {
		t.Fatal("a successful resolution carries no resolution identity")
	}
	if !result.Anchor().At().Equal(anchorAt) || result.Anchor().PolicyVersion().String() != "anchor-policy-v1" {
		t.Fatal("result did not echo the selection anchor it resolved under")
	}
}

// Covers: AT-PC-020, AT-PC-021, AT-PC-027 — 零、多与读取失败是三个不同结果，任何一个
// 都不得由系统任选一条或伪装成客户不合格。
func TestZeroMultipleAndUnavailableAreDistinctOutcomes(t *testing.T) {
	t.Run("zero candidates is no applicable basis", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-other")

		result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject))
		if result.Outcome() != domain.NoApplicableBasis {
			t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", result.Outcome())
		}
		if _, present := result.AdoptedVersion(); present {
			t.Fatal("a no-basis result named an adopted version")
		}
	})

	t.Run("two candidates conflict rather than picking the higher version", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-2", "v9", "sha256:c2", "scope-a")

		result := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject))
		if result.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
		}
		if _, present := result.AdoptedVersion(); present {
			t.Fatal("a conflict picked a winner")
		}
		if result.CandidateCount() != 2 {
			t.Fatalf("candidate count = %d, want 2", result.CandidateCount())
		}
	})

	t.Run("an unreadable authority view is pending, not no-basis", func(t *testing.T) {
		result := domain.ResolveCommercialBasis(nil, resolutionKey(t, "scope-a", domain.CustomerContractObject))
		if result.Outcome() != domain.ResolutionPending {
			t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
		}
	})
}

// Covers: AT-PC-018 — 商业选择锚点策略未配置时解析未决，绝不退回系统当前时间。
func TestMissingAnchorPolicyIsPendingRatherThanDefaultingToNow(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	if _, err := domain.NewSelectionAnchor(anchorAt, domain.AnchorPolicyVersion{}); err == nil {
		t.Fatal("an anchor without its policy version constructed")
	}

	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	key.Anchor = domain.SelectionAnchor{}

	result := domain.ResolveCommercialBasis(registry, key)
	if result.Outcome() != domain.ResolutionPending {
		t.Fatalf("outcome = %q, want RESOLUTION_PENDING", result.Outcome())
	}
	if _, present := result.AdoptedVersion(); present {
		t.Fatal("resolution proceeded without a selection anchor")
	}
}

// Covers: UC-PC-002 输入未受理 — 最小身份不成立时不查询、不泄露候选。
func TestIncompleteKeyIsNotAcceptedWithoutTouchingCandidates(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	incomplete := map[string]func(*domain.ResolutionKey){
		"no tenant":        func(key *domain.ResolutionKey) { key.TenantID = domain.TenantID{} },
		"no customer":      func(key *domain.ResolutionKey) { key.CustomerAccountID = domain.CustomerAccountID{} },
		"no legal entity":  func(key *domain.ResolutionKey) { key.LegalEntityCandidate = domain.LegalEntityReference{} },
		"no scope":         func(key *domain.ResolutionKey) { key.Scope = domain.CommercialScopeReference{} },
		"no required kind": func(key *domain.ResolutionKey) { key.RequiredBasis = domain.CommercialObjectKindInvalid },
	}

	for name, breakKey := range incomplete {
		t.Run(name, func(t *testing.T) {
			key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
			breakKey(&key)

			result := domain.ResolveCommercialBasis(registry, key)
			if result.Outcome() != domain.InputNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if result.CandidateCount() != 0 {
				t.Fatal("an unaccepted input still counted candidates, which leaks their existence")
			}
		})
	}
}

// Covers: party-commercial CONTEXT 已生效 → 已到期/已退役/已替代 停止用于新的解析 —
// 收尾过的版本不再是候选，且这不同于「从来没有」。
func TestEndedVersionsLeaveTheCandidateSet(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	live := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")

	retired, err := live.Retire(commercialValue(t, domain.NewRetirementReference, "retire-1"), anchorAt.AddDate(0, -1, 0))
	if err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := domain.NewCommercialRegistry().Register(retired); err != nil {
		t.Fatalf("register retired: %v", err)
	}

	retiredOnly := domain.NewCommercialRegistry()
	if _, err := retiredOnly.Register(retired); err != nil {
		t.Fatalf("register retired: %v", err)
	}

	result := domain.ResolveCommercialBasis(retiredOnly, resolutionKey(t, "scope-a", domain.CustomerContractObject))
	if result.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", result.Outcome())
	}
}

// Covers: AT-PC-024 — 相同输入与相同权威视图重复解析返回相同语义，不产生新商业版本。
func TestRepeatedResolutionIsStableAndCreatesNothing(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)

	first := domain.ResolveCommercialBasis(registry, key)
	second := domain.ResolveCommercialBasis(registry, key)

	if first.Outcome() != second.Outcome() || first.ResolutionID() != second.ResolutionID() {
		t.Fatalf("repeated resolution diverged: %q/%q vs %q/%q",
			first.Outcome(), first.ResolutionID(), second.Outcome(), second.ResolutionID())
	}
	if registry.Count() != 1 {
		t.Fatalf("resolving created commercial versions: registry holds %d", registry.Count())
	}
}
