package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件把渠道择优编排接到 `parcel-pricing` 的批量评价口上（票 `label-channel/12` 接线）。
//
// 它必须落在适配器包：架构门禁只许适配器同时导入两个上下文，而编排住在应用层。也就是说
// 「一份输入对多份 `BUY` 价卡逐份评价」这件事，编排看不见也不该看见——它只经
// `ports.ChannelCandidateCostSource` 拿逐候选的成本。

// ErrPricingInputNotConfigured 说这一票折不出计价输入。
//
// 停下而不是拿一份空输入去评价：计价输入的取数路径（尺寸重量从哪来、区域怎么定）是消费方
// 自己的实例半边，今天没有租户就没有它。一份编出来的输入会让每个候选都算出一个看着合法的
// 价格，而那些价格背后没有任何真实包裹。
var ErrPricingInputNotConfigured = errors.New("parcel shipment: pricing input source not configured")

// PricingInputSource 把一次择优折成计价输入快照。
//
// 第二个返回值为 false 即「未配置」，与 CommercialBasisAdapter 的 keys 同一手法：缺席如实
// 交出来，不代拟一份。
type PricingInputSource interface {
	PricingInputFor(
		ctx context.Context,
		query psports.ChannelSelectionQuery,
	) (ppdomain.PricingInputSnapshot, bool, error)
}

// ChannelBuyPlanSource 给出某个渠道候选的 `BUY` 价卡版本与本次评价的标识。
//
// found=false 是「这个候选没有登记过报价表」，不是错误：`ADR-0088` 要求每家渠道产品的报价表
// 逐份登记，没登记的候选按提供方 CONTEXT 属`待判断`——补一张卡就能出价。
type ChannelBuyPlanSource interface {
	BuyPlanFor(
		ctx context.Context,
		query psports.ChannelSelectionQuery,
		candidate psdomain.ChannelCandidateID,
	) (ppdomain.PlanEvaluationTarget, bool, error)
}

// ChannelCandidateCostDeps 收拢两个取数口，两者都是消费方的实例半边。
type ChannelCandidateCostDeps struct {
	Input PricingInputSource
	Plans ChannelBuyPlanSource
}

// ChannelCandidateCostAdapter 为一批渠道候选逐个取回成本单维取值。
type ChannelCandidateCostAdapter struct {
	input PricingInputSource
	plans ChannelBuyPlanSource
}

func NewChannelCandidateCostAdapter(deps ChannelCandidateCostDeps) *ChannelCandidateCostAdapter {
	return &ChannelCandidateCostAdapter{input: deps.Input, plans: deps.Plans}
}

var _ psports.ChannelCandidateCostSource = (*ChannelCandidateCostAdapter)(nil)

// ChannelCandidateCosts 逐格作答：交回的条数与入参候选一一对应、次序相同。
//
// 没有登记价卡的候选带着`待判断`回来而不是从结果里消失——消失与「参选了没赢」在下游完全
// 同形，而它其实一分钱的价都没算过。
func (adapter *ChannelCandidateCostAdapter) ChannelCandidateCosts(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	candidates []psdomain.ChannelCandidateID,
) ([]psdomain.ChannelCandidateCost, error) {
	input, configured, err := adapter.input.PricingInputFor(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("form pricing input: %w", err)
	}
	if !configured {
		return nil, ErrPricingInputNotConfigured
	}

	// 先把有卡的候选收成一批交给批量口，「逐候选各算各的计费重」由它保证；没卡的候选不进
	// 这一批，它们不是算不出而是压根没有可算之物。
	carded := make([]ppdomain.PlanEvaluationTarget, 0, len(candidates))
	cardedOf := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		target, found, err := adapter.plans.BuyPlanFor(ctx, query, candidate)
		if err != nil {
			return nil, fmt.Errorf("load buy plan for %s: %w", candidate.String(), err)
		}
		if !found {
			continue
		}
		cardedOf[candidate.String()] = len(carded)
		carded = append(carded, target)
	}

	evaluations := ppdomain.EvaluatePricingAcrossPlans(input, ppdomain.EvidenceSynthetic, carded)
	if len(evaluations) != len(carded) {
		// 批量口的契约是等长对位。它不成立时手上这批评价接不回候选，只能上抛。
		return nil, fmt.Errorf("%w: %d evaluations for %d targets",
			ErrUntranslatableEvaluation, len(evaluations), len(carded))
	}

	costs := make([]psdomain.ChannelCandidateCost, 0, len(candidates))
	for _, candidate := range candidates {
		index, carded := cardedOf[candidate.String()]
		if !carded {
			cost, err := unpriceable(candidate, psdomain.ChannelCostPendingEvidence)
			if err != nil {
				return nil, err
			}
			costs = append(costs, cost)
			continue
		}
		cost, err := ChannelCandidateCostOf(candidate, evaluations[index])
		if err != nil {
			return nil, err
		}
		costs = append(costs, cost)
	}
	return costs, nil
}
