package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: AT-PS-166；ADR-0295 决定一——解开冲突的去向是封闭两值，词不在集合内即拒（含大小写变体与空串），不猜；决定人与依据是必填引用，空白即拒。
func TestTheConflictDecisionKindsAreAClosedPair(t *testing.T) {
	for raw, want := range map[string]domain.CancellationOrderConflictDecisionKind{
		"CANCELLATION_NOT_UPHELD": domain.CancellationNotUpheld,
		"BOUNDARY_FACT_REFERRED":  domain.BoundaryFactReferred,
	} {
		got, err := domain.ParseCancellationOrderConflictDecisionKind(raw)
		if err != nil || got != want || got.String() != raw {
			t.Errorf("%q -> %v %v, want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "cancellation_not_upheld", "DELETE_THE_CANCELLATION", " BOUNDARY_FACT_REFERRED"} {
		if got, err := domain.ParseCancellationOrderConflictDecisionKind(raw); err == nil || got != domain.CancellationOrderConflictDecisionKindInvalid {
			t.Errorf("%q 被认成了 %v", raw, got)
		}
	}
	if domain.CancellationOrderConflictDecisionKindInvalid.String() != "" {
		t.Error("无效值有名字")
	}
	if _, err := domain.NewCancellationOrderConflictDeciderReference(" "); !errors.Is(err, domain.ErrBlankValue) {
		t.Errorf("空白决定人：%v", err)
	}
	if _, err := domain.NewCancellationOrderConflictDecisionBasis(""); !errors.Is(err, domain.ErrBlankValue) {
		t.Errorf("空白依据：%v", err)
	}
}

// Covers: AT-PS-166, AT-PS-171；边界种类的词只有一张表：接入面与存储读回都经它认词，词不在封闭集内即拒。
func TestTheBoundaryKindWordsAreAClosedPair(t *testing.T) {
	for _, kind := range []domain.CancellationBoundaryKind{domain.NetworkIntakeBoundary, domain.CarrierFirstEffectivePickupBoundary} {
		if got, err := domain.ParseCancellationBoundaryKind(kind.String()); err != nil || got != kind {
			t.Errorf("%q -> %v %v", kind, got, err)
		}
	}
	for _, raw := range []string{"", "SCAN", "network_intake"} {
		if _, err := domain.ParseCancellationBoundaryKind(raw); err == nil {
			t.Errorf("%q 被认成了边界种类", raw)
		}
	}
}

// Covers: AT-PS-171；ADR-0295 决定二——这一代收寄已被认定有误、交来源方更正：带着收寄比出冲突的那一代落在 ReferredBoundaries 里，答`取消在先`而不是`不形成`/冲突；
// 引用的是另一代、或没有这份名单，答冲突。
func TestAReferredPickupGenerationNoLongerStandsAsAConflict(t *testing.T) {
	conflicting := labelServiceInput(t, emptyRegister(t))
	conflicting.FirstEffectivePickup = firstPickup(t)
	conflicting.Cancellation = standingCancellationAt(t, labelServicePickupAt.Add(time.Hour))

	referredRef := mustValue(t, domain.NewCancellationBoundaryReference, "TF-TRACK-FACT-7@TF-TRACK-FACT-7/v1")
	otherRef := mustValue(t, domain.NewCancellationBoundaryReference, "TF-TRACK-FACT-7@TF-TRACK-FACT-7/v2")

	for name, referred := range map[string][]domain.CancellationBoundaryReference{
		"没有名单":     nil,
		"名单上是另一代":  {otherRef},
		"名单上有这一代":  {otherRef, referredRef},
		"名单上只有这一代": {referredRef},
	} {
		input := conflicting
		input.ReferredBoundaries = referred
		verdict := judge(t, input)
		wantReferred := name == "名单上有这一代" || name == "名单上只有这一代"
		if wantReferred {
			if verdict.Judgment() != domain.LabelServiceCancellationStands {
				t.Errorf("%s：judgment = %s, want CANCELLATION_STANDS", name, verdict.Judgment())
			}
			if _, conflicted := verdict.Conflict(); conflicted {
				t.Errorf("%s：已被交来源方更正的一代又判出了新冲突", name)
			}
			continue
		}
		if verdict.Judgment() != domain.LabelServiceNotFinal || verdict.Reason() != domain.LabelServiceCancellationOrderConflict {
			t.Errorf("%s：verdict = %s/%s, want NOT_FINAL/CANCELLATION_ORDER_CONFLICT", name, verdict.Judgment(), verdict.Reason())
		}
	}
}
