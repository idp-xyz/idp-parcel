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

// Handle 选出这一票该走的渠道。
func (handler *SelectChannelCandidateHandler) Handle(
	ctx context.Context,
	query ports.ChannelSelectionQuery,
) (domain.ChannelCandidateID, error) {
	candidates, err := handler.deps.Assembly.AssembleChannelCandidates(ctx, query)
	if err != nil {
		return domain.ChannelCandidateID{}, fmt.Errorf("assemble channel candidates: %w", err)
	}

	costs, err := handler.deps.Costs.ChannelCandidateCosts(ctx, query, candidates)
	if err != nil {
		return domain.ChannelCandidateID{}, fmt.Errorf("read channel candidate costs: %w", err)
	}
	if err := coversEveryCandidate(candidates, costs); err != nil {
		return domain.ChannelCandidateID{}, err
	}

	selected, err := domain.SelectChannelCandidateByCost(costs)
	if err != nil && !errors.Is(err, domain.ErrChannelCandidateCostTied) &&
		!errors.Is(err, domain.ErrNoQualifiedChannelCandidate) {
		// 币种不齐（或日后其它「没比出来」的出口）：那一次没有比较发生，没有决定可记。
		return domain.ChannelCandidateID{}, err
	}
	if recordErr := handler.recordDecision(ctx, query, costs); recordErr != nil {
		return domain.ChannelCandidateID{}, recordErr
	}
	return selected, err
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
