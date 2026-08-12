package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func reviewedPlan(t *testing.T) domain.InitialRoutePlan {
	t.Helper()
	plan, err := domain.FormInitialRoutePlan(planSpec(t))
	if err != nil {
		t.Fatalf("form plan: %v", err)
	}
	return plan
}

// Covers: UC-NR-003 复核层次 2「实际起点/节点……是否与计划相容」与层次 3「限制……自动
// 和人工都不能绕过」——位置不在计划上或选中候选被限制都成确定性失效，依据各自指得回
// 实际位置与限制来源（它就是 Lapse 的输入）；事实齐备且无失效依据时仍适用，不造新计划。
func TestAPlanReviewLapsesByMismatchOrRestriction(t *testing.T) {
	plan := reviewedPlan(t)

	offPlan, err := domain.ReviewPlanApplicability(plan,
		mustValue(t, domain.NewActualLocationReference, "node-elsewhere"), nil)
	if err != nil {
		t.Fatalf("review off-plan: %v", err)
	}
	if offPlan.Outcome() != domain.PlanNoLongerApplicable {
		t.Fatalf("outcome = %q, want NO_LONGER_APPLICABLE", offPlan.Outcome())
	}
	basis, present := offPlan.LapseBasis()
	if !present || basis.String() != "PLAN_ORIGIN_MISMATCH/node-elsewhere" {
		t.Fatalf("basis = %v present = %v", basis, present)
	}

	restricted, err := domain.ReviewPlanApplicability(plan,
		mustValue(t, domain.NewActualLocationReference, "node-origin"),
		[]domain.HardConstraintFinding{restrictionOn(t, "cand-1")})
	if err != nil {
		t.Fatalf("review restricted: %v", err)
	}
	if restricted.Outcome() != domain.PlanNoLongerApplicable {
		t.Fatalf("outcome = %q, want NO_LONGER_APPLICABLE", restricted.Outcome())
	}
	if basis, _ := restricted.LapseBasis(); basis.String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/CC-REF-9" {
		t.Fatalf("basis = %v; 失效依据必须指得回限制来源", basis)
	}

	applicable, err := domain.ReviewPlanApplicability(plan,
		mustValue(t, domain.NewActualLocationReference, "node-hub"), nil)
	if err != nil {
		t.Fatalf("review applicable: %v", err)
	}
	if applicable.Outcome() != domain.PlanStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE——不制造语义重复的新计划", applicable.Outcome())
	}
}

// Covers: 结果语义「当前计划适用性未决——禁止默认保留、默认失效」与两条判断线的分界：
// 选中候选限制状态未知停未决带缺口；**别的**候选的限制推翻不了这份计划（那是候选评估线
// 的输入）。
func TestAPlanReviewStaysUndecidedOnlyForItsOwnCandidate(t *testing.T) {
	plan := reviewedPlan(t)
	origin := mustValue(t, domain.NewActualLocationReference, "node-origin")

	undecided, err := domain.ReviewPlanApplicability(plan, origin,
		[]domain.HardConstraintFinding{unknownConstraintOn(t, "cand-1")})
	if err != nil {
		t.Fatalf("review undecided: %v", err)
	}
	if undecided.Outcome() != domain.PlanReviewUndecided {
		t.Fatalf("outcome = %q, want REVIEW_UNDECIDED", undecided.Outcome())
	}
	gap, present := undecided.Gap()
	if !present || gap.Reference().String() != "DANGEROUS_GOODS_CLASSIFICATION" {
		t.Fatalf("gap = %#v present = %v", gap, present)
	}

	foreign, err := domain.ReviewPlanApplicability(plan, origin,
		[]domain.HardConstraintFinding{
			restrictionOn(t, "cand-9"),
			unknownConstraintOn(t, "cand-8"),
		})
	if err != nil {
		t.Fatalf("review foreign constraints: %v", err)
	}
	if foreign.Outcome() != domain.PlanStillApplicable {
		t.Fatalf("outcome = %q; 别的候选的限制推翻了这份计划", foreign.Outcome())
	}

	if _, err := domain.ReviewPlanApplicability(plan, domain.ActualLocationReference{}, nil); !errors.Is(err, domain.ErrInvalidPlanReview) {
		t.Fatalf("err = %v, want ErrInvalidPlanReview", err)
	}
}

func restrictionOn(t *testing.T, candidate string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate:   mustValue(t, domain.NewCandidateID, candidate),
		Outcome:     domain.RestrictionApplies,
		Restriction: mustValue(t, domain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/CC-REF-9"),
	})
	if err != nil {
		t.Fatalf("new restriction finding: %v", err)
	}
	return finding
}

func unknownConstraintOn(t *testing.T, candidate string) domain.HardConstraintFinding {
	t.Helper()
	finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: mustValue(t, domain.NewCandidateID, candidate),
		Outcome:   domain.ConstraintStatusUnknown,
		Missing:   mustValue(t, domain.NewEvidenceGapReference, "DANGEROUS_GOODS_CLASSIFICATION"),
		Reassess:  mustValue(t, domain.NewReassessmentCondition, "WHEN_GOODS_CLASSIFICATION_CONFIRMED"),
	})
	if err != nil {
		t.Fatalf("new unknown finding: %v", err)
	}
	return finding
}
