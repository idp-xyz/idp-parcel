package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件把渠道择优的三段串起来（票 `label-channel/12` 接线那一步）：装配出候选、逐候选取
// 成本、按成本单维择优。
//
// 三段的规则各归各家——收窄归 `party-commercial`、评价归 `parcel-pricing`、出局与并列归本
// 上下文的比较器。本编排一条业务规则都不加，它只负责**不在段与段之间掉东西**。

// ErrChannelCostsIncomplete 说取成本口没有为每一个装配出的候选逐格作答。
//
// 停下而不是拿手上这些接着比：比较器从一个被削过的集合里照样选得出「最便宜的」并且一路绿，
// 而少掉的那个可能恰好最便宜。集合被削这件事在结果上留不下任何痕迹，所以只能在这里挡。
var ErrChannelCostsIncomplete = errors.New("parcel shipment: channel costs do not cover every assembled candidate")

// ErrChannelSelectionRecordingMisconfigured 说留痕三件（登记口、标识签发口、时钟）只装了一半。
//
// 响亮失败而不是静默不记：静默不记与「没配登记」在结果上同形，而装配缺件是要修的。
var ErrChannelSelectionRecordingMisconfigured = errors.New("parcel shipment: channel selection decision recording is half configured")

// SelectChannelCandidateDeps 收拢两个出向口，以及留痕那一组（票 `label-channel/14`）。
//
// 留痕三件**一起可缺席**：Decisions 为 nil 时编排行为与此前一字不变，既有装配点不必知道这一格
// 存在；只到一半则是装配缺件（ErrChannelSelectionRecordingMisconfigured）。
type SelectChannelCandidateDeps struct {
	Assembly ports.ChannelCandidateAssembly
	Costs    ports.ChannelCandidateCostSource

	// Decisions 是「渠道择优决定」的只追加登记册。择优落定后在**同一事务**里写一条：三种
	// 非错误出口（选出、并列冲突、无人参选）各成一条；没有比较发生的停下（成本表对不上、
	// 币种不齐）不成记录。
	Decisions   ports.ChannelSelectionDecisionRegistry
	DecisionIDs ports.ChannelSelectionDecisionIdentity
	Clock       ports.Clock
}

type SelectChannelCandidateHandler struct {
	deps SelectChannelCandidateDeps
}

func NewSelectChannelCandidateHandler(deps SelectChannelCandidateDeps) *SelectChannelCandidateHandler {
	return &SelectChannelCandidateHandler{deps: deps}
}

// ChannelSelectionOutcome 是择优编排的结果代数，按调用方的恢复动作分格（ADR-0029；票 `label-channel/35` 做法二）。
// 三格都是**比较发生过、决定已记**的业务答案——票 14 把并列与无人参选叫「非错误出口」，票 01 裁决把并列定为
// 「交人裁」的正当结果；它们此前长在 `error` 上，每个调用方都得从真错误里把这两个挑出来，分类在三处各写一遍。
// 没比出来的停下（成本表不全、币种不齐、留痕半配、依赖故障）不在这张表上，仍是 error，成因由各口具名。
type ChannelSelectionOutcome uint8

const (
	ChannelSelectionOutcomeInvalid ChannelSelectionOutcome = iota
	// ChannelSelectionSelected：选出唯一一条，Selected() 在场；决定记 SELECTED。
	ChannelSelectionSelected
	// ChannelSelectionCostTied：最低价并列、选不出唯一一条，等人裁（票 01 / `PAR-NET-16`）；决定记 TIED。
	ChannelSelectionCostTied
	// ChannelSelectionNoQualifiedCandidate：无人参选；决定已记。续办是补价卡或放宽约束，不是重试。
	ChannelSelectionNoQualifiedCandidate
)

func (outcome ChannelSelectionOutcome) String() string {
	switch outcome {
	case ChannelSelectionSelected:
		return "SELECTED"
	case ChannelSelectionCostTied:
		return "COST_TIED"
	case ChannelSelectionNoQualifiedCandidate:
		return "NO_QUALIFIED_CANDIDATE"
	default:
		return ""
	}
}

// ChannelSelectionResult 交回择优停在哪一格，连同选中的候选（只有 ChannelSelectionSelected 那一格在场）。
type ChannelSelectionResult struct {
	outcome     ChannelSelectionOutcome
	selected    domain.SelectedChannelCandidate
	hasSelected bool
}

func (result ChannelSelectionResult) Outcome() ChannelSelectionOutcome {
	return result.outcome
}

// Selected 交回选中的候选，连同它按之出价的评价痕迹与费率引用；并列冲突与无人参选时缺席。
func (result ChannelSelectionResult) Selected() (domain.SelectedChannelCandidate, bool) {
	return result.selected, result.hasSelected
}

// Select 选出这一票该走的渠道，连同赢家按之出价的评价痕迹与费率引用（票 `label-channel/29`：建立面单
// 交易时 Rate 那一格由择优步带出，翻译适配器不再问 `parcel-pricing`）。两格从 Costs 口交回的那一份
// 取值上取，不另取——另取一遍就有了第二份「按哪张卡出的价」。
//
// 比较器的两个非选中出口（并列、无人参选）在这里译成结果格，`error` 只留真错误（票 35 做法二）：领域比较器
// `SelectChannelCandidateByCost` 的这两个出口与决定册的结论三格一一对应，它们在领域里已经是「结果」，只是此前
// 在编排的交回值上长成了「错误」。这是那两个领域哨兵**唯一**的翻译点——调用方与组合根不再认它们。
func (handler *SelectChannelCandidateHandler) Select(
	ctx context.Context,
	query ports.ChannelSelectionQuery,
) (ChannelSelectionResult, error) {
	none := ChannelSelectionResult{}
	candidates, err := handler.deps.Assembly.AssembleChannelCandidates(ctx, query)
	if err != nil {
		return none, fmt.Errorf("assemble channel candidates: %w", err)
	}

	costs, err := handler.deps.Costs.ChannelCandidateCosts(ctx, query, candidates)
	if err != nil {
		return none, fmt.Errorf("read channel candidate costs: %w", err)
	}
	if err := coversEveryCandidate(candidates, costs); err != nil {
		return none, err
	}

	winner, err := domain.SelectChannelCandidateByCost(costs)
	var outcome ChannelSelectionOutcome
	switch {
	case err == nil:
		outcome = ChannelSelectionSelected
	case errors.Is(err, domain.ErrChannelCandidateCostTied):
		outcome = ChannelSelectionCostTied
	case errors.Is(err, domain.ErrNoQualifiedChannelCandidate):
		outcome = ChannelSelectionNoQualifiedCandidate
	default:
		// 币种不齐（或日后其它「没比出来」的出口）：那一次没有比较发生，没有决定可记。
		return none, err
	}
	if recordErr := handler.recordDecision(ctx, query, costs); recordErr != nil {
		return none, recordErr
	}
	if outcome != ChannelSelectionSelected {
		return ChannelSelectionResult{outcome: outcome}, nil
	}
	selected, err := selectedCandidateOf(winner, costs)
	if err != nil {
		return none, err
	}
	return ChannelSelectionResult{outcome: ChannelSelectionSelected, selected: selected, hasSelected: true}, nil
}

// selectedCandidateOf 把赢家与它在成本表上的那一份取值对上，带出评价痕迹与费率。找不到那一份是编排
// 自己的矛盾（赢家正是从这张表里比出来的），按成本表不全报。
func selectedCandidateOf(
	candidate domain.ChannelCandidateID,
	costs []domain.ChannelCandidateCost,
) (domain.SelectedChannelCandidate, error) {
	selected, err := domain.NewSelectedChannelCandidate(candidate)
	if err != nil {
		return domain.SelectedChannelCandidate{}, fmt.Errorf("form selected channel candidate: %w", err)
	}
	for _, cost := range costs {
		if cost.Candidate() != candidate {
			continue
		}
		if evaluation, present := cost.Evaluation(); present {
			if selected, err = selected.WithEvaluation(evaluation); err != nil {
				return domain.SelectedChannelCandidate{}, fmt.Errorf("attach evaluation to selected candidate: %w", err)
			}
		}
		if rate, present := cost.Rate(); present {
			if selected, err = selected.WithRate(rate); err != nil {
				return domain.SelectedChannelCandidate{}, fmt.Errorf("attach rate to selected candidate: %w", err)
			}
		}
		return selected, nil
	}
	return domain.SelectedChannelCandidate{}, fmt.Errorf("%w: selected %s has no cost entry", ErrChannelCostsIncomplete, candidate.String())
}

// recordDecision 把刚比过的这一批记成一条决定。四格由 FormChannelSelectionDecision 从同一份排序
// 判出，编排不给结论——给了就有第二个口径。登记口写不进去即择优不算落定：同一事务里两者要么
// 一起成立、要么一起消失。
func (handler *SelectChannelCandidateHandler) recordDecision(
	ctx context.Context,
	query ports.ChannelSelectionQuery,
	costs []domain.ChannelCandidateCost,
) error {
	deps := handler.deps
	if deps.Decisions == nil && deps.DecisionIDs == nil && deps.Clock == nil {
		return nil
	}
	if deps.Decisions == nil || deps.DecisionIDs == nil || deps.Clock == nil {
		return ErrChannelSelectionRecordingMisconfigured
	}

	id, err := deps.DecisionIDs.NextChannelSelectionDecisionID(ctx)
	if err != nil {
		return fmt.Errorf("issue channel selection decision ID: %w", err)
	}
	subject, err := domain.NewChannelSelectionSubject(query.Scope, query.Mapping)
	if err != nil {
		return fmt.Errorf("form channel selection subject: %w", err)
	}
	decision, err := domain.FormChannelSelectionDecision(domain.ChannelSelectionDecisionSpec{
		ID:            id,
		Tenant:        query.Tenant,
		Subject:       subject,
		AssembledAsOf: query.At,
		DecidedAt:     deps.Clock.Now(),
		Costs:         costs,
	})
	if err != nil {
		return fmt.Errorf("form channel selection decision: %w", err)
	}
	if _, err := deps.Decisions.Append(ctx, decision); err != nil {
		return fmt.Errorf("record channel selection decision: %w", err)
	}
	return nil
}

// coversEveryCandidate 核对成本表逐格答过每一个候选。
//
// 比身份不比条数：条数相等也可能是「漏了一个、又多出一个从没装配过的」，而那种错法恰好
// 是条数核不出来的那一种。
func coversEveryCandidate(
	candidates []domain.ChannelCandidateID,
	costs []domain.ChannelCandidateCost,
) error {
	answered := make(map[domain.ChannelCandidateID]struct{}, len(costs))
	for _, cost := range costs {
		answered[cost.Candidate()] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, found := answered[candidate]; !found {
			return fmt.Errorf("%w: %s", ErrChannelCostsIncomplete, candidate.String())
		}
	}
	return nil
}
