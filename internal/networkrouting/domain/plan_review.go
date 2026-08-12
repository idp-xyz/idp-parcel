package domain

import "errors"

var ErrInvalidPlanReview = errors.New("network routing: invalid plan review facts")

// PlanReviewOutcome 是当前计划适用性复核的封闭三值（UC-NR-003 步骤 6A：独立评估当前
// 计划是否仍可执行，不等待候选排序）。`适用性未决`单独一格：分不清「计划坏了」与
// 「还看不清」时默认保留是免检放行、默认失效是凭空下线，硬句把两个默认都禁了。
type PlanReviewOutcome uint8

const (
	PlanReviewOutcomeInvalid PlanReviewOutcome = iota
	PlanStillApplicable
	PlanNoLongerApplicable
	PlanReviewUndecided
)

func (outcome PlanReviewOutcome) String() string {
	switch outcome {
	case PlanStillApplicable:
		return "STILL_APPLICABLE"
	case PlanNoLongerApplicable:
		return "NO_LONGER_APPLICABLE"
	case PlanReviewUndecided:
		return "REVIEW_UNDECIDED"
	default:
		return ""
	}
}

// PlanReview 是一次适用性复核的结果：失效必带依据（它就是 PlanApplicability.Lapse 的
// 输入），未决必带缺口（缺什么、何时再判）。
type PlanReview struct {
	outcome PlanReviewOutcome
	basis   ApplicabilityBasisReference
	gap     EvidenceGap
	hasGap  bool
}

func (review PlanReview) Outcome() PlanReviewOutcome {
	return review.outcome
}

// LapseBasis 只在`不再适用`时给出——没有依据的失效与拍脑袋下线分不开。
func (review PlanReview) LapseBasis() (ApplicabilityBasisReference, bool) {
	return review.basis, review.basis.valid()
}

// Gap 只在`适用性未决`时给出，携带缺少内容与再次判断条件。
func (review PlanReview) Gap() (EvidenceGap, bool) {
	return review.gap, review.hasGap
}

// ReviewPlanApplicability 独立复核当前计划是否仍可执行（复核层次 2 与 3 对计划本体的
// 那一半）：
//
//   - 实际接货/控制位置不在计划节点序列上，计划与实际不相容——确定性失效，依据携带
//     实际位置（「实际起点/节点……是否与计划相容」）；
//   - 计划选中候选被有效硬限制淘汰——确定性失效，依据携带限制来源（自动和人工都不能
//     绕过）；
//   - 选中候选的限制状态未知——适用性未决，缺口原样交回：默认保留是免检放行，默认
//     失效是凭空下线；
//   - 事实齐备且没有失效依据——仍适用，不制造语义重复的新计划。
//
// 它只看这份计划自己的可执行性，不比较替代候选——那是另一条独立判断线（6B），两条线
// 的结论按硬句各自成立。
func ReviewPlanApplicability(
	plan InitialRoutePlan,
	actual ActualLocationReference,
	constraints []HardConstraintFinding,
) (PlanReview, error) {
	if !actual.valid() {
		return PlanReview{}, ErrInvalidPlanReview
	}

	onPlan := false
	for _, node := range plan.Nodes() {
		if node.String() == actual.String() {
			onPlan = true
			break
		}
	}
	if !onPlan {
		basis, err := NewApplicabilityBasisReference("PLAN_ORIGIN_MISMATCH/" + actual.String())
		if err != nil {
			return PlanReview{}, err
		}
		return PlanReview{outcome: PlanNoLongerApplicable, basis: basis}, nil
	}

	for _, finding := range constraints {
		if !finding.candidate.valid() || !finding.outcome.valid() {
			return PlanReview{}, ErrInvalidPlanReview
		}
		if finding.candidate != plan.SelectedCandidate() {
			// 别的候选的限制推翻不了这份计划——那是候选评估线（6B）的输入。
			continue
		}
		switch finding.outcome {
		case RestrictionApplies:
			basis, err := NewApplicabilityBasisReference(
				"HARD_CONSTRAINT_RESTRICTION/" + finding.restriction.String())
			if err != nil {
				return PlanReview{}, err
			}
			return PlanReview{outcome: PlanNoLongerApplicable, basis: basis}, nil
		case ConstraintStatusUnknown:
			gap, err := NewEvidenceGap(
				finding.missing,
				CandidateScopedGap,
				[]CandidateID{finding.candidate},
				finding.reassess,
			)
			if err != nil {
				return PlanReview{}, err
			}
			return PlanReview{outcome: PlanReviewUndecided, gap: gap, hasGap: true}, nil
		}
	}
	return PlanReview{outcome: PlanStillApplicable}, nil
}
