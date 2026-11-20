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
// MeasurementSource 是恰好一条实际测量事实引用时那次测量的来源身份；没有，或多于一条时为空。重量与来源只在评价上
// 有一份，费用行与调整都读这一份，不另拷。
//
// 评价没有测量事实时来源三格缺席；申报没报尺寸时尺寸缺席。
type PricedWeightAndOrigin struct {
	Kind              string
	PricedWeight      string
	PricedWeightUnit  string
	ActualWeight      string
	ActualWeightUnit  string
	DimensionSides    []string
	DimensionUnit     string
	MeasurementSource string
}

// MustTranslateEvaluationRefs 是费用行自己的评价引用：计价一侧造不出标识就整批报错，不交回。
type MustTranslateEvaluationRefs []string

// AbsentIfUntranslatableEvaluationRefs 是调整上的评价引用：造不出标识的那一个缺席，其余照常交回。
type AbsentIfUntranslatableEvaluationRefs []string

// PricedWeightsAndOrigins 空标识与重复标识不问；登记册里没有的评价在答复里缺席。必须译出的那一组里，计价一侧造不出
// 评价标识的引用报 ErrUntranslatableReference 并点名它，整批不交回。读不出就缺席的那一组里，造不出标识只让这一个
// 缺席。同一条引用两组都有时必须译出的那一组先走，造不出标识就不会被缺席吞掉。两组并成一次问登记册。
func (origins *PricedWeightsAndOrigins) PricedWeightsAndOrigins(
	ctx context.Context,
	tenant sadomain.TenantID,
	mustTranslate MustTranslateEvaluationRefs,
	absentIfUntranslatable AbsentIfUntranslatableEvaluationRefs,
) (map[string]PricedWeightAndOrigin, error) {
	pricingTenant, err := ppdomain.NewTenantID(tenant.String())
	if err != nil {
		return nil, err
	}
	ids := make([]ppdomain.EvaluationID, 0, len(mustTranslate)+len(absentIfUntranslatable))
	seen := map[string]struct{}{}
	// 必须译出的一组先走：同一条引用也在从宽组里时，造不出标识要报错，不能先被从宽组跳成缺席。
	for _, raw := range mustTranslate {
		id, include, err := evaluationIDToAsk(raw, seen)
		if err != nil {
			// 费用行上的评价引用只来自读得回评价的那条形成路径，计价一侧造不出标识的引用是数据坏了；跳过它会把一笔
			// 说不清缘由的费用答成「没有计价重量」，与评价确实没有测量事实混在一起。
			return nil, fmt.Errorf("%w: evaluation reference %q: %v", ErrUntranslatableReference, raw, err)
		}
		if include {
			ids = append(ids, id)
		}
	}
	for _, raw := range absentIfUntranslatable {
		id, include, err := evaluationIDToAsk(raw, seen)
		if err != nil {
			// 调整上的评价引用是结算侧的自由文本，手工计价纠错可以写出计价一侧造不出的标识。这一笔重量与来源缺席，
			// 不把同页其余费用答成未形成。
			continue
		}
		if include {
			ids = append(ids, id)
		}
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

func evaluationIDToAsk(raw string, seen map[string]struct{}) (ppdomain.EvaluationID, bool, error) {
	if raw == "" {
		return ppdomain.EvaluationID{}, false, nil
	}
	if _, ok := seen[raw]; ok {
		return ppdomain.EvaluationID{}, false, nil
	}
	id, err := ppdomain.NewEvaluationID(raw)
	if err != nil {
		return ppdomain.EvaluationID{}, false, err
	}
	seen[raw] = struct{}{}
	return id, true, nil
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
		origin.MeasurementSource = source.MeasurementSource
		answered = true
	}
	return origin, answered
}
