package parcelpricing_test

import (
	"context"
	"testing"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	bridge "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/parcelpricing"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证成本取数口（票 `label-channel/12` 接线那一步的另一半）：把一批渠道候选逐个折成
// 成本单维取值，交给择优编排。
//
// 它是 `ports.ChannelCandidateCostSource` 的实现，而那个端口的契约里最硬的一条是**逐格作答**
// ——交回的条数与入参候选一一对应。这里证的就是它，以及「答不上来的那一格答的是什么」。

type stubPricingInput struct {
	input      ppdomain.PricingInputSnapshot
	configured bool
}

func (stub stubPricingInput) PricingInputFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
) (ppdomain.PricingInputSnapshot, bool, error) {
	return stub.input, stub.configured, nil
}

// stubBuyPlans 按候选标识给出它的 `BUY` 价卡；表里没有即「这个候选没有登记过报价表」。
type stubBuyPlans struct {
	plans map[string]ppdomain.PlanEvaluationTarget
}

func (stub stubBuyPlans) BuyPlanFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
	candidate psdomain.ChannelCandidateID,
) (ppdomain.PlanEvaluationTarget, bool, error) {
	target, found := stub.plans[candidate.String()]
	return target, found, nil
}

func planTargetFor(t testing.TB, id string, plan ppdomain.PricingPlanVersion) ppdomain.PlanEvaluationTarget {
	t.Helper()

	target, err := ppdomain.NewPlanEvaluationTarget(mustValue(t, ppdomain.NewEvaluationID, id), plan)
	if err != nil {
		t.Fatalf("构造批量评价目标 %s：%v", id, err)
	}
	return target
}

func costSelectionQuery(t testing.TB) psports.ChannelSelectionQuery {
	t.Helper()

	return psports.ChannelSelectionQuery{
		Tenant:  mustValue(t, psdomain.NewTenantID, "tenant-1"),
		Scope:   mustValue(t, psdomain.NewCommercialScopeReference, "scope-a"),
		Mapping: mustValue(t, psdomain.NewProductChannelMappingReference, "mapping-1"),
		At:      at(t, businessDay),
	}
}

// Covers: `ports.ChannelCandidateCostSource` 的「逐格作答」契约——**没有登记 `BUY` 价卡的候选
// 要带着出局格回来，而不是从结果里消失**。
//
// 消失是这一层最贵的错：它与「那个候选参选了但没赢」在下游完全同形，而择优编排核对的是
// 身份覆盖，一旦这里少答一格编排就会停下——那是好的；真正危险的是**这里自作主张把它填成
// 别的东西**。所以这一格既钉条数也钉那一格的内容。
//
// 出局格取`待判断`而非`不可计价`，依据是提供方 CONTEXT 的原话「取不到价卡、区间空档、事实
// 缺失属待判断，不属不可计价」——补一张卡就能出价，续办是去登记，不是换渠道。
func TestACandidateWithNoRegisteredBuyCardComesBackPendingRatherThanVanishing(t *testing.T) {
	t.Parallel()

	priced := buyPlan(t, "cost-source-priced", "12.00", nil, ppdomain.PricingPlanStructures{})
	adapter := bridge.NewChannelCandidateCostAdapter(bridge.ChannelCandidateCostDeps{
		Input: stubPricingInput{input: pricingInput(t, "Z1", at(t, businessDay), nil), configured: true},
		Plans: stubBuyPlans{plans: map[string]ppdomain.PlanEvaluationTarget{
			"cand-carded": planTargetFor(t, "eval-cand-carded", priced),
		}},
	})

	candidates := []psdomain.ChannelCandidateID{
		candidate(t, "cand-carded"),
		candidate(t, "cand-uncarded"),
	}
	costs, err := adapter.ChannelCandidateCosts(context.Background(), costSelectionQuery(t), candidates)
	if err != nil {
		t.Fatalf("取成本：%v", err)
	}

	if len(costs) != 2 {
		t.Fatalf("成本条数 = %d，want 2——没有价卡的候选从结果里消失了", len(costs))
	}
	// 次序与入参对位：编排按身份核对覆盖，错位比少一条更难发现。
	if costs[0].Candidate().String() != "cand-carded" || costs[1].Candidate().String() != "cand-uncarded" {
		t.Fatalf("成本次序 = [%s %s]，want [cand-carded cand-uncarded]",
			costs[0].Candidate().String(), costs[1].Candidate().String())
	}

	if _, established := costs[0].Amount(); !established {
		t.Fatalf("有价卡的候选没算出成本，问题在夹具不在被测件")
	}

	grade, unavailable := costs[1].Unavailability()
	if !unavailable || grade != psdomain.ChannelCostPendingEvidence {
		t.Fatalf("没有价卡的候选出局格 = %s（未确立 = %v），want %s",
			grade, unavailable, psdomain.ChannelCostPendingEvidence)
	}
}
