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
	outcome         StageReviewOutcome
	decision        domain.StageReviewDecision
	hasRecord       bool
	conflicts       []domain.AuthorityConflict
	intervalPending string
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

// IntervalContinuation 非空说明决定已落库但授予的权威区间还没追加成功：决定与区间
// 分岔正是本模块点名要防的双帐，续办引用让重放路把区间补上（同意图纪律——决定不翻，
// 只重试同一份追加）。
func (result RecordStageReviewResult) IntervalContinuation() string {
	return result.intervalPending
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
		result := RecordStageReviewResult{
			outcome:   ReviewExistingDecision,
			decision:  existing,
			hasRecord: true,
		}
		// 重放路补追加：上次区间追加失败留下的分岔在这里收口——决定不翻，只重试
		// 同一份追加（同 handOff 纪律的 existingResult 重发）。
		if command.GrantedInterval != nil {
			result.intervalPending = handler.appendInterval(ctx, *command.GrantedInterval)
		}
		return result, nil
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

	result := RecordStageReviewResult{
		outcome:   ReviewRecorded,
		decision:  decision,
		hasRecord: true,
	}
	if command.GrantedInterval != nil {
		result.intervalPending = handler.appendInterval(ctx, *command.GrantedInterval)
	}
	return result, nil
}

// appendInterval 追加权威区间。失败不翻已落库的决定（原子性同等事务闸门），但必须
// 交回续办引用——决定在册而区间缺失且无处可知，正是权威不明的双帐分岔；重放路凭
// 同一命令重试同一份追加。已在册的区间（重放补追加撞上已成功的上次）不是失败。
func (handler *RecordStageReviewHandler) appendInterval(
	ctx context.Context,
	interval domain.AuthorityInterval,
) string {
	current, err := handler.deps.Intervals.ListCurrent(ctx)
	if err == nil {
		for _, existing := range current {
			if existing == interval {
				return ""
			}
		}
	}
	if err := handler.deps.Intervals.Append(ctx, interval); err != nil {
		return "CONT-INTERVAL/" + interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind
	}
	return ""
}
