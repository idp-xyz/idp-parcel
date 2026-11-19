package parcelpricing

import (
	"context"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PricedWeightsAndOrigins 按评价标识读回计价重量，连同评价用的实重、尺寸与它们的来源。这些都在评价上，费用行只留评价标识。
type PricedWeightsAndOrigins struct {
	evaluations evaluationsByID
}

// evaluationsByID 是评价登记册按一批标识读的那一口。
type evaluationsByID interface {
	FindByIDs(context.Context, ppdomain.TenantID, []ppdomain.EvaluationID) ([]ppdomain.PricingEvaluation, error)
}

func NewPricedWeightsAndOrigins(evaluations evaluationsByID) (*PricedWeightsAndOrigins, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting: priced weights and origins are missing the evaluation register")
	}
	return &PricedWeightsAndOrigins{evaluations: evaluations}, nil
}

// PricedWeightAndOrigin 是一笔费用的计价重量与它的来源，两样分开答。
//
// PricedWeight 是评价进位之后、据以查表的计价重量，与接受前估价留痕记的是同一个。ActualWeight 与 DimensionSides
// 是评价用的实重与尺寸（三边由长到短），Kind 说这两样出自实测（actual-measurement）还是申报（declared-measurement）。
// 实重不能顶替计价重量：取较大者的卡上，复测只把尺寸量大了，实重一字不变，计价重量与金额照样会变。
//
// 评价没有测量事实时来源三格缺席；申报没报尺寸时尺寸缺席。
type PricedWeightAndOrigin struct {
	Kind             string
	PricedWeight     string
	PricedWeightUnit string
	ActualWeight     string
	ActualWeightUnit string
	DimensionSides   []string
	DimensionUnit    string
}

// PricedWeightsAndOrigins 空标识与重复标识不问；登记册里没有的评价在答复里缺席。计价一侧造不出评价标识的引用报
// ErrUntranslatableReference 并点名它，整批不交回。
func (origins *PricedWeightsAndOrigins) PricedWeightsAndOrigins(
	ctx context.Context,
	tenant sadomain.TenantID,
	evaluationIDs []string,
) (map[string]PricedWeightAndOrigin, error) {
	pricingTenant, err := ppdomain.NewTenantID(tenant.String())
	if err != nil {
		return nil, err
	}
	ids := make([]ppdomain.EvaluationID, 0, len(evaluationIDs))
	seen := map[string]struct{}{}
	for _, raw := range evaluationIDs {
		if raw == "" {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		id, err := ppdomain.NewEvaluationID(raw)
		if err != nil {
			// 费用行上的评价引用只来自读得回评价的那条形成路径，计价一侧造不出标识的引用是数据坏了；跳过它会把一笔
			// 说不清缘由的费用答成「没有计价重量」，与评价确实没有测量事实混在一起。
			return nil, fmt.Errorf("%w: evaluation reference %q: %v", ErrUntranslatableReference, raw, err)
		}
		ids = append(ids, id)
	}
	found, err := origins.evaluations.FindByIDs(ctx, pricingTenant, ids)
	if err != nil {
		return nil, err
	}
	answered := map[string]PricedWeightAndOrigin{}
	for _, evaluation := range found {
		if origin, ok := pricedWeightAndOriginOf(evaluation); ok {
			answered[evaluation.ID().String()] = origin
		}
	}
	return answered, nil
}

// pricedWeightAndOriginOf 从一份评价里读出计价重量与来源。两样都读不出时第二个返回值为假，费用册上这一格空着。
func pricedWeightAndOriginOf(evaluation ppdomain.PricingEvaluation) (PricedWeightAndOrigin, bool) {
	var origin PricedWeightAndOrigin
	answered := false
	if result, present := evaluation.PricingWeight(); present {
		rounded := result.RoundedWeight()
		origin.PricedWeight, origin.PricedWeightUnit = rounded.Value().String(), rounded.Unit().String()
		answered = true
	}
	if source, ok := ppdomain.WeightAndDimensionsOriginOf(evaluation.Input()); ok {
		origin.Kind = string(source.Kind)
		origin.ActualWeight, origin.ActualWeightUnit = source.ActualWeight.Value().String(), source.ActualWeight.Unit().String()
		if source.HasDimensions {
			sides := source.Dimensions
			origin.DimensionSides = []string{
				sides.LongestSide().Value().String(),
				sides.SecondLongestSide().Value().String(),
				sides.ShortestSide().Value().String(),
			}
			origin.DimensionUnit = sides.Unit().String()
		}
		answered = true
	}
	return origin, answered
}
