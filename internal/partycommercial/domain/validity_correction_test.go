package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: `AT-PC-013`「外部源更正历史有效区间 → 保留原版本和更正关系，推进修订标识」。
//
// 与 Revise / ErrCommercialContentIsFixed 分清：这是区间更正关系，不是改正文（ADR-0038）。
func TestValidityCorrectionKeepsOriginalAndAdvancesViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	live := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-1", "scope-a")
	originalInterval := live.Effective()
	before := registry.ViewRevision(live.Scope())

	atAnchor := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if atAnchor.Outcome() != domain.UniquelyResolved {
		t.Fatalf("before correction outcome = %q, want UNIQUELY_RESOLVED", atAnchor.Outcome())
	}

	correctedInterval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new corrected interval: %v", err)
	}
	correctionRef := commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-1")
	correction, err := live.CorrectEffectiveInterval(correctedInterval, correctionRef, time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("correct effective interval: %v", err)
	}

	after, err := registry.RegisterValidityCorrection(correction)
	if err != nil {
		t.Fatalf("register validity correction: %v", err)
	}
	if after.String() == before.String() {
		t.Fatal("有效性更正后 ViewRevision 没有推进")
	}

	stored, found := registry.Lookup(live.Kind(), live.ObjectID(), live.Version())
	if !found {
		t.Fatal("更正后原版本从登记册消失了")
	}
	if stored.ContentDigest() != live.ContentDigest() {
		t.Fatal("更正改写了原版本正文")
	}
	if stored.Effective() != originalInterval {
		t.Fatal("更正覆盖了原版本上记录的历史区间")
	}
	basis, present := stored.ApprovalBasis()
	originalBasis, _ := live.ApprovalBasis()
	if !present || basis != originalBasis {
		t.Fatal("更正改写了原批准判断")
	}

	held, ok := registry.ValidityCorrectionOf(live.Kind(), live.ObjectID(), live.Version())
	if !ok {
		t.Fatal("更正关系没有留下来")
	}
	if held.Reference() != correctionRef {
		t.Fatalf("correction reference = %q, want %q", held.Reference(), correctionRef)
	}
	if held.CorrectedInterval() != correctedInterval {
		t.Fatal("更正后区间没有按外部源落下")
	}

	// anchorAt = 2026-06-01：落在原区间内、更正后区间外 → 不得再适用。
	afterResolve := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if afterResolve.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("after correction outcome = %q, want NO_APPLICABLE_BASIS", afterResolve.Outcome())
	}
	if afterRev, ok := afterResolve.ViewRevision(); !ok || afterRev.String() != after.String() {
		t.Fatal("解析结果没有带上更正后的修订标识")
	}

	t.Run("draft cannot be validity-corrected", func(t *testing.T) {
		draft := commercialDraft(t, domain.CustomerContractObject, "contract-2", "v1", "sha256:c2")
		_, err := draft.CorrectEffectiveInterval(correctedInterval, correctionRef, time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
		if !errors.Is(err, domain.ErrInvalidCommercialTransition) {
			t.Fatalf("error = %v, want ErrInvalidCommercialTransition", err)
		}
	})

	t.Run("published content still cannot be revised in place", func(t *testing.T) {
		if _, err := live.Revise(commercialValue(t, domain.NewCommercialContentDigest, "sha256:rewritten")); !errors.Is(err, domain.ErrCommercialContentIsFixed) {
			t.Fatalf("error = %v, want ErrCommercialContentIsFixed", err)
		}
	})
}
