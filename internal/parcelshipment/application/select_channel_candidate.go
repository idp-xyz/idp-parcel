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

// SelectChannelCandidateDeps 收拢两个出向口。
type SelectChannelCandidateDeps struct {
	Assembly ports.ChannelCandidateAssembly
	Costs    ports.ChannelCandidateCostSource
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

	return domain.SelectChannelCandidateByCost(costs)
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
