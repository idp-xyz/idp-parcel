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
	before := registry.ViewRevision(live.Tenant(), live.Scope())

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

	stored, found := registry.Lookup(live.Tenant(), live.Kind(), live.ObjectID(), live.Version())
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

	held, ok := registry.ValidityCorrectionOf(live.Tenant(), live.Kind(), live.ObjectID(), live.Version())
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

// Covers: D-5「更正一条更正」合法；两侧同为只增多条，选用区间取登记顺序最后一条。
//
// 第一条把 6 月锚点踢出选用区间；第二条把区间拉回覆盖锚点。覆盖那一次会让第一条消失，
// 解析仍答无适用依据；只增则第二条成为选用，解析回到唯一已解析，原版本一字不动。
func TestASecondValidityCorrectionIsAppendedAndSelected(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	live := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-1", "scope-a")
	originalInterval := live.Effective()

	firstInterval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("first interval: %v", err)
	}
	secondInterval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("second interval: %v", err)
	}
	first, err := live.CorrectEffectiveInterval(
		firstInterval,
		commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-1"),
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("first correction: %v", err)
	}
	second, err := live.CorrectEffectiveInterval(
		secondInterval,
		commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-2"),
		time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("second correction: %v", err)
	}

	afterFirst, err := registry.RegisterValidityCorrection(first)
	if err != nil {
		t.Fatalf("register first: %v", err)
	}
	afterSecond, err := registry.RegisterValidityCorrection(second)
	if err != nil {
		t.Fatalf("register second: %v", err)
	}
	if afterSecond.String() == afterFirst.String() {
		t.Fatal("第二条更正没有推进 ViewRevision")
	}

	stored, found := registry.Lookup(live.Tenant(), live.Kind(), live.ObjectID(), live.Version())
	if !found {
		t.Fatal("第二条更正后原版本从登记册消失了")
	}
	if stored.Effective() != originalInterval {
		t.Fatal("第二条更正覆盖了原版本上记录的历史区间")
	}

	held, ok := registry.ValidityCorrectionOf(live.Tenant(), live.Kind(), live.ObjectID(), live.Version())
	if !ok {
		t.Fatal("选用更正读不回来")
	}
	if held.Reference() != second.Reference() {
		t.Fatal("选用更正不是登记顺序上的最后一条")
	}
	if held.CorrectedInterval() != secondInterval {
		t.Fatal("选用区间不是最后一条更正后的区间")
	}

	afterResolve := domain.ResolveCommercialBasis(registry, resolutionKey(t, "scope-a", domain.CustomerContractObject), nil)
	if afterResolve.Outcome() != domain.UniquelyResolved {
		t.Fatalf("after second correction outcome = %q, want UNIQUELY_RESOLVED", afterResolve.Outcome())
	}

	t.Run("replaying the last correction does not advance the view", func(t *testing.T) {
		replay, err := registry.RegisterValidityCorrection(second)
		if err != nil {
			t.Fatalf("replay last: %v", err)
		}
		if replay.String() != afterSecond.String() {
			t.Fatal("重放最后一条更正却推进了 ViewRevision")
		}
	})

	t.Run("replaying an earlier correction does not undo the last", func(t *testing.T) {
		replay, err := registry.RegisterValidityCorrection(first)
		if err != nil {
			t.Fatalf("replay first: %v", err)
		}
		if replay.String() != afterSecond.String() {
			t.Fatal("重放较早一条更正却推进了 ViewRevision")
		}
		held, ok := registry.ValidityCorrectionOf(live.Tenant(), live.Kind(), live.ObjectID(), live.Version())
		if !ok || held.Reference() != second.Reference() {
			t.Fatal("重放较早一条把选用更正改回了它")
		}
	})
}

// Covers: D-5「ViewRevision 覆盖全部更正历史」。只哈希最后一条的话，单独登记 B 与先 A 再 B
// 会得到同一个修订，一次已被更正覆盖的历史就从失效检测里消失。
func TestValidityCorrectionHistoryParticipatesInViewRevision(t *testing.T) {
	onlyLast := domain.NewCommercialRegistry()
	both := domain.NewCommercialRegistry()
	liveOnly := effectiveIn(t, onlyLast, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-1", "scope-a")
	liveBoth := effectiveIn(t, both, domain.CustomerContractObject, "contract-1", "v1", "sha256:contract-1", "scope-a")

	firstInterval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("first interval: %v", err)
	}
	secondInterval, err := domain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("second interval: %v", err)
	}

	first, err := liveBoth.CorrectEffectiveInterval(
		firstInterval,
		commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-1"),
		time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("first correction: %v", err)
	}
	secondBoth, err := liveBoth.CorrectEffectiveInterval(
		secondInterval,
		commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-2"),
		time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("second correction: %v", err)
	}
	secondOnly, err := liveOnly.CorrectEffectiveInterval(
		secondInterval,
		commercialValue(t, domain.NewValidityCorrectionReference, "ext-correction-2"),
		time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("only-last correction: %v", err)
	}

	if _, err := both.RegisterValidityCorrection(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if _, err := both.RegisterValidityCorrection(secondBoth); err != nil {
		t.Fatalf("register second: %v", err)
	}
	if _, err := onlyLast.RegisterValidityCorrection(secondOnly); err != nil {
		t.Fatalf("register only last: %v", err)
	}

	if onlyLast.ViewRevision(liveOnly.Tenant(), liveOnly.Scope()) == both.ViewRevision(liveBoth.Tenant(), liveBoth.Scope()) {
		t.Fatal("只含最后一条与含全部历史的登记册得到同一个 ViewRevision")
	}
}
