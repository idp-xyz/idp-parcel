package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// FixCandidateSetOutcome 是固定候选版本组的应用处理结果。
type FixCandidateSetOutcome uint8

const (
	FixCandidateSetOutcomeInvalid FixCandidateSetOutcome = iota
	CandidateSetFixed
	CandidateSetAlreadyFixed
	CandidateSetContentConflict
	CandidateSetNotAccepted
	CandidateSetUndecided
)

func (outcome FixCandidateSetOutcome) String() string {
	switch outcome {
	case CandidateSetFixed:
		return "FIXED"
	case CandidateSetAlreadyFixed:
		return "ALREADY_FIXED"
	case CandidateSetContentConflict:
		return "CONTENT_CONFLICT"
	case CandidateSetNotAccepted:
		return "NOT_ACCEPTED"
	case CandidateSetUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type FixCandidateSetResult struct {
	outcome FixCandidateSetOutcome
}

func (result FixCandidateSetResult) Outcome() FixCandidateSetOutcome {
	return result.outcome
}

// FixCandidateSetHandler 固定一次阶段评审要评的候选版本组。评审引用的组必须已在册
// （RecordStageReviewHandler 的候选组引用完整性），这里是它进册的那一步。
//
// 组不可扩张（domain.CandidateVersionSet）：同标识同内容是重放，同标识异内容拒——范围扩大
// 或版本变更要换一个新组，不改这一组。
type FixCandidateSetHandler struct {
	sets ports.CandidateSetStore
}

func NewFixCandidateSetHandler(sets ports.CandidateSetStore) *FixCandidateSetHandler {
	return &FixCandidateSetHandler{sets: sets}
}

func (handler *FixCandidateSetHandler) Handle(
	ctx context.Context,
	set domain.CandidateVersionSet,
) (FixCandidateSetResult, error) {
	if set.ID().String() == "" {
		// 零值组只可能来自没过 domain.FixCandidateVersionSet 的调用方。
		return FixCandidateSetResult{outcome: CandidateSetNotAccepted}, nil
	}
	existing, found, err := handler.sets.FindByID(ctx, set.ID())
	if err != nil {
		return FixCandidateSetResult{outcome: CandidateSetUndecided}, nil
	}
	if found {
		if sameCandidateSet(existing, set) {
			return FixCandidateSetResult{outcome: CandidateSetAlreadyFixed}, nil
		}
		return FixCandidateSetResult{outcome: CandidateSetContentConflict}, nil
	}
	if err := handler.sets.Save(ctx, set); err != nil {
		// 并发下另一方先固定了同一组，或存储故障：这一刻答不出落没落，重跑同一命令即得确定答案。
		return FixCandidateSetResult{outcome: CandidateSetUndecided}, nil
	}
	return FixCandidateSetResult{outcome: CandidateSetFixed}, nil
}

func sameCandidateSet(existing, declared domain.CandidateVersionSet) bool {
	return existing.ID() == declared.ID() &&
		existing.Scope() == declared.Scope() &&
		existing.Parameters() == declared.Parameters() &&
		existing.Rules() == declared.Rules() &&
		existing.FormedAt().Equal(declared.FormedAt())
}
