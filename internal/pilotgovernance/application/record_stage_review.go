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
	CoverageConflictBlocked
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
	case CoverageConflictBlocked:
		return "COVERAGE_CONFLICT"
	default:
		return ""
	}
}

// RecordStageReviewCommand 携带一次阶段评审的全部输入。GrantedInterval 只在 Go 进
// 限量生产时在场——那一刻要对精确范围建立唯一权威区间。Coverage 是随本次决定一并
// 登记的覆盖关系声明：后继一律是本次评审的范围版本，关系不设独立登记路——无
// Go/No-Go 决定就无关系登记。
type RecordStageReviewCommand struct {
	Review          domain.StageReviewDecisionSpec
	GrantedInterval *domain.AuthorityInterval
	Coverage        []domain.ScopeCoverageDeclaration
}

type RecordStageReviewResult struct {
	outcome           StageReviewOutcome
	decision          domain.StageReviewDecision
	hasRecord         bool
	conflicts         []domain.AuthorityConflict
	coverageConflicts []domain.ScopeVersionRelationConflict
	intervalPending   string
	coveragePending   string
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

// CoverageConflicts 只在覆盖关系相悖阻断时给出——处置者要知道声明撞上了在册的哪条边。
func (result RecordStageReviewResult) CoverageConflicts() []domain.ScopeVersionRelationConflict {
	return append([]domain.ScopeVersionRelationConflict(nil), result.coverageConflicts...)
}

// CoverageContinuation 非空说明决定已落库但覆盖关系边还没全部登上。分岔期间第三态
// 照旧保守作答（关系读不出来即拦着，不是放行），续办引用让重放路把边补上。
func (result RecordStageReviewResult) CoverageContinuation() string {
	return result.coveragePending
}

type RecordStageReviewDeps struct {
	Candidates ports.CandidateSetStore
	Reviews    ports.ReviewDecisionStore
	Intervals  ports.AuthorityIntervalStore
	// Relations 只在命令带覆盖关系声明时被触碰；不带声明的评审照旧不依赖它。
	Relations ports.ScopeVersionRelationStore
	Clock     ports.Clock
}

type RecordStageReviewHandler struct {
	deps RecordStageReviewDeps
}

func NewRecordStageReviewHandler(deps RecordStageReviewDeps) *RecordStageReviewHandler {
	return &RecordStageReviewHandler{deps: deps}
}

// Handle 记录一次不可覆盖的阶段评审：候选组引用完整性 → 覆盖关系声明构造门 → 幂等
// （同目标同候选组只决定一次）→ 权威区间冲突预检 → 覆盖关系相悖预检（两道预检都
// 先于任何落库——「双写后人工对账」是被点名的错误结果，相悖边静默收下则是绕过恢复
// 决定的旁路）→ 领域构造（Go/No-Go 形状约束在域）→ 提交 → 追加区间与关系边。
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

	relations, err := coverageRelations(command)
	if err != nil {
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
		// 重放路补追加：上次区间追加或关系边登记失败留下的分岔在这里收口——决定
		// 不翻，只重试同一份追加（同 handOff 纪律的 existingResult 重发）。
		if command.GrantedInterval != nil {
			result.intervalPending = handler.appendInterval(ctx, *command.GrantedInterval)
		}
		result.coveragePending = handler.saveCoverage(ctx, relations)
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

	if len(relations) > 0 {
		conflicts, err := handler.findCoverageConflicts(ctx, relations)
		if err != nil {
			return RecordStageReviewResult{outcome: ReviewUndecided}, nil
		}
		if len(conflicts) > 0 {
			return RecordStageReviewResult{
				outcome:           CoverageConflictBlocked,
				coverageConflicts: conflicts,
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
	result.coveragePending = handler.saveCoverage(ctx, relations)
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

// coverageRelations 把命令里的覆盖关系声明构造成边：后继一律是本次评审的范围版本，
// 所属决定引用取本次评审的（目标 + 候选组），登记时点取决定时点。声明装不成边、或
// 同一命令内两条声明彼此不独立（同前代重复或相悖）都未受理——一次决定对同一对版本
// 只能说一句话。
func coverageRelations(command RecordStageReviewCommand) ([]domain.ScopeVersionRelation, error) {
	relations := make([]domain.ScopeVersionRelation, 0, len(command.Coverage))
	for _, declaration := range command.Coverage {
		relation, err := domain.RegisterScopeVersionRelation(domain.ScopeVersionRelationSpec{
			Successor:    command.Review.Scope,
			Predecessor:  declaration.Predecessor,
			Kind:         declaration.Kind,
			Objective:    command.Review.Objective,
			Candidates:   command.Review.Candidates,
			RegisteredAt: command.Review.DecidedAt,
		})
		if err != nil {
			return nil, err
		}
		for _, earlier := range relations {
			if domain.CompareScopeVersionRelations(earlier, relation) != domain.RelationComparisonInvalid {
				return nil, domain.ErrInvalidScopeRelation
			}
		}
		relations = append(relations, relation)
	}
	return relations, nil
}

// findCoverageConflicts 对每条声明查同对与反向对的在册边，交回全部相悖对。相悖必须
// 在决定落库前被拦下：后到的决定改写不了先到的登记，静默收下等于让关系登记变成绕过
// 恢复决定的旁路。
func (handler *RecordStageReviewHandler) findCoverageConflicts(
	ctx context.Context,
	relations []domain.ScopeVersionRelation,
) ([]domain.ScopeVersionRelationConflict, error) {
	conflicts := make([]domain.ScopeVersionRelationConflict, 0)
	for _, declared := range relations {
		for _, pair := range [][2]domain.ScopeVersionReference{
			{declared.Successor(), declared.Predecessor()},
			{declared.Predecessor(), declared.Successor()},
		} {
			existing, found, err := handler.deps.Relations.FindByPair(ctx, pair[0], pair[1])
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			if domain.CompareScopeVersionRelations(existing, declared) == domain.RelationsContradictory {
				conflicts = append(conflicts, domain.ScopeVersionRelationConflict{
					Declared: declared,
					Existing: existing,
				})
			}
		}
	}
	return conflicts, nil
}

// saveCoverage 把覆盖关系边落册。失败不翻已落库的决定，但必须交回续办引用——决定在
// 册而边缺失期间，第三态照旧保守作答（拦着，不是放行），重放路凭同一命令补登同一份。
// 「同一事实已在册」（重放撞上已成功的上次，或对称的互不相干已从另一方向登过）不是
// 失败；重放时撞上相悖边（决定已落，拦不回去了）同样走续办引用留给人工。
func (handler *RecordStageReviewHandler) saveCoverage(
	ctx context.Context,
	relations []domain.ScopeVersionRelation,
) string {
	for _, relation := range relations {
		pending := "CONT-COVERAGE/" + relation.Successor().String() + "/" + relation.Predecessor().String()
		redundant := false
		for _, pair := range [][2]domain.ScopeVersionReference{
			{relation.Successor(), relation.Predecessor()},
			{relation.Predecessor(), relation.Successor()},
		} {
			existing, found, err := handler.deps.Relations.FindByPair(ctx, pair[0], pair[1])
			if err != nil {
				return pending
			}
			if !found {
				continue
			}
			switch domain.CompareScopeVersionRelations(existing, relation) {
			case domain.RelationRedundant:
				redundant = true
			case domain.RelationsContradictory:
				return pending
			}
		}
		if redundant {
			continue
		}
		if _, err := handler.deps.Relations.Save(ctx, relation); err != nil {
			return pending
		}
	}
	return ""
}
