// Package application 编排 pilotgovernance 的治理用例。判据在领域，这里只做受理、
// 幂等、冲突预检与提交的协调。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// ErrUnexpectedReviewSave 说明决定库交回了封闭集合以外的写入结果。
var ErrUnexpectedReviewSave = errors.New("pilot governance: unexpected review save outcome")

// StageReviewOutcome 是记录阶段评审的应用处理结果。
type StageReviewOutcome uint8

const (
	StageReviewOutcomeInvalid StageReviewOutcome = iota
	ReviewRecorded
	ReviewExistingDecision
	AuthorityConflictBlocked
	ReviewNotAccepted
	ReviewUndecided
)

func (outcome StageReviewOutcome) String() string {
	switch outcome {
	case ReviewRecorded:
		return "RECORDED"
	case ReviewExistingDecision:
		return "EXISTING_DECISION"
	case AuthorityConflictBlocked:
		return "AUTHORITY_CONFLICT"
	case ReviewNotAccepted:
		return "NOT_ACCEPTED"
	case ReviewUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RecordStageReviewCommand 携带一次阶段评审的全部输入。GrantedInterval 只在 Go 进
// 限量生产时在场——那一刻要对精确范围建立唯一权威区间。
type RecordStageReviewCommand struct {
	Review          domain.StageReviewDecisionSpec
	GrantedInterval *domain.AuthorityInterval
}

type RecordStageReviewResult struct {
	outcome   StageReviewOutcome
	decision  domain.StageReviewDecision
	hasRecord bool
	conflicts []domain.AuthorityConflict
}

func (result RecordStageReviewResult) Outcome() StageReviewOutcome {
	return result.outcome
}

func (result RecordStageReviewResult) Decision() (domain.StageReviewDecision, bool) {
	return result.decision, result.hasRecord
}

// Conflicts 只在权威冲突阻断时给出——处置者要知道撞上了哪些区间，一对都不能少。
func (result RecordStageReviewResult) Conflicts() []domain.AuthorityConflict {
	return append([]domain.AuthorityConflict(nil), result.conflicts...)
}

type RecordStageReviewDeps struct {
	Candidates ports.CandidateSetStore
	Reviews    ports.ReviewDecisionStore
	Intervals  ports.AuthorityIntervalStore
	Clock      ports.Clock
}

type RecordStageReviewHandler struct {
	deps RecordStageReviewDeps
}

func NewRecordStageReviewHandler(deps RecordStageReviewDeps) *RecordStageReviewHandler {
	return &RecordStageReviewHandler{deps: deps}
}

// Handle 记录一次不可覆盖的阶段评审：候选组引用完整性 → 幂等（同目标同候选组只
// 决定一次）→ 权威区间冲突预检（重叠即阻断带全部冲突对，先于任何落库——「双写后
// 人工对账」是被点名的错误结果）→ 领域构造（Go/No-Go 形状约束在域）→ 提交。
func (handler *RecordStageReviewHandler) Handle(
	ctx context.Context,
	command RecordStageReviewCommand,
) (RecordStageReviewResult, error) {
	_, found, err := handler.deps.Candidates.FindByID(ctx, command.Review.Candidates)
	if err != nil {
		return RecordStageReviewResult{outcome: ReviewUndecided}, nil
	}
	if !found {
		// 引用不存在的候选组：决定的范围身份悬空，改请求而不是重试。
		return RecordStageReviewResult{outcome: ReviewNotAccepted}, nil
	}

	key := ports.ReviewKey{Objective: command.Review.Objective, Candidates: command.Review.Candidates}
	existing, found, err := handler.deps.Reviews.FindByKey(ctx, key)
	if err != nil {
		return RecordStageReviewResult{outcome: ReviewUndecided}, nil
	}
	if found {
		return RecordStageReviewResult{
			outcome:   ReviewExistingDecision,
			decision:  existing,
			hasRecord: true,
		}, nil
	}

	if command.GrantedInterval != nil {
		current, err := handler.deps.Intervals.ListCurrent(ctx)
		if err != nil {
			return RecordStageReviewResult{outcome: ReviewUndecided}, nil
		}
		conflicts, err := domain.DetectAuthorityConflicts(append(current, *command.GrantedInterval))
		if err != nil {
			return RecordStageReviewResult{outcome: ReviewNotAccepted}, nil
		}
		if len(conflicts) > 0 {
			return RecordStageReviewResult{
				outcome:   AuthorityConflictBlocked,
				conflicts: conflicts,
			}, nil
		}
	}

	decision, err := domain.RecordStageReview(command.Review)
	if err != nil {
		return RecordStageReviewResult{outcome: ReviewNotAccepted}, nil
	}

	saved, err := handler.deps.Reviews.Save(ctx, key, decision)
	if err != nil {
		return RecordStageReviewResult{outcome: ReviewUndecided}, nil
	}
	switch saved {
	case ports.ReviewSaved:
	case ports.ReviewAlreadyRecorded:
		winner, found, err := handler.deps.Reviews.FindByKey(ctx, key)
		if err != nil || !found {
			return RecordStageReviewResult{outcome: ReviewUndecided}, nil
		}
		return RecordStageReviewResult{
			outcome:   ReviewExistingDecision,
			decision:  winner,
			hasRecord: true,
		}, nil
	default:
		return RecordStageReviewResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReviewSave, saved)
	}

	if command.GrantedInterval != nil {
		// 区间追加失败不翻已落库的决定——决定与区间的原子性同样等事务闸门，此处
		// 与其余上下文的意图纪律一致：结果已成立，续办只补区间。
		_ = handler.deps.Intervals.Append(ctx, *command.GrantedInterval)
	}
	return RecordStageReviewResult{
		outcome:   ReviewRecorded,
		decision:  decision,
		hasRecord: true,
	}, nil
}
