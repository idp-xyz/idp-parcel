package parcelpricing

import (
	"errors"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本包是渠道择优的成本分值桥（票 `label-channel/13` 缝二）。位置由 ADR-0025 定死：只有
// 消费侧的 `internal/<consumer>/adapters/<provider>/` 可以同时导入两个上下文，
// `TestBusinessModulesDoNotReachIntoEachOther` 守着这一条。
//
// **它只翻译，不判断。** 「算不出就出局、且不得以零金额顶替」是择优的规则，票 `01` 已裁给
// 渠道择优比较器那一层；桥在这里做出局判断就成了第二处口径，而两处口径迟早各自演化。
// 桥的全部职责是把提供方的答复如实换成本上下文的说法：算得出，或者算不出、是哪一格。

var (
	// ErrUntranslatableEvaluation 说提供方交回了本桥认不出的取值。
	//
	// 逐格分派不留兜底（ADR-0031）：认不出就上抛，不静默落进某一格。计价侧日后新增第六种
	// 评价结果时，这里必须红——否则那一格会悄悄被读成`未形成`，而`未形成`的续办是重试，
	// 对一个其实需要人工裁决的新结果来说，重试是永远不会停的。
	ErrUntranslatableEvaluation = errors.New("parcel shipment: untranslatable pricing evaluation")
	// ErrNotASupplierCostEvaluation 说这份评价答的不是「运营企业花多少」。
	//
	// 价格方向隔离是提供方 CONTEXT 的明文规则，本桥据以拒绝而不是翻译：一份 `SELL` 评价
	// 结构完全合法、金额齐备，译过去一路绿，下游没有任何东西还能看出这个数字答的是
	// 「向客户收多少」。而渠道择优拿它选出的，是一笔真实的供应商采购。
	ErrNotASupplierCostEvaluation = errors.New("parcel shipment: pricing evaluation is not a supplier cost")
)

// ChannelCandidateCostOf 把一个渠道候选的 `BUY` 评价译成它在成本单维上的取值。
//
// 全函数：提供方的五种评价结果各有去处，没有一种会让桥交回零值加 nil。已完成译成已确立
// 成本，其余四种各译到自己的出局格——四格不合并的理由写在 psdomain.ChannelCostUnavailability
// 上，此处不重述。
func ChannelCandidateCostOf(
	candidate psdomain.ChannelCandidateID,
	evaluation ppdomain.PricingEvaluation,
) (psdomain.ChannelCandidateCost, error) {
	if evaluation.Direction() != ppdomain.PricingDirectionBuy ||
		evaluation.Purpose() != ppdomain.PricingPurposeSupplierCost {
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: direction %q purpose %q",
			ErrNotASupplierCostEvaluation, evaluation.Direction(), evaluation.Purpose())
	}

	switch status := evaluation.Status(); status {
	case ppdomain.EvaluationCompleted:
		return establishedCost(candidate, evaluation)
	case ppdomain.EvaluationPending:
		return unpriceable(candidate, psdomain.ChannelCostPendingEvidence)
	case ppdomain.EvaluationUnratable:
		return unpriceable(candidate, psdomain.ChannelCostRatecardExclusion)
	case ppdomain.EvaluationConflict:
		return unpriceable(candidate, psdomain.ChannelCostConflict)
	case ppdomain.EvaluationFailed:
		return unpriceable(candidate, psdomain.ChannelCostNotFormed)
	default:
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: evaluation status %q",
			ErrUntranslatableEvaluation, status)
	}
}

// establishedCost 译一份已完成评价。金额与币种照评价原样过去，不折成最小币单位——理由
// 写在 psdomain.ChannelCostAmount 上。
func establishedCost(
	candidate psdomain.ChannelCandidateID,
	evaluation ppdomain.PricingEvaluation,
) (psdomain.ChannelCandidateCost, error) {
	total, priced := evaluation.Total()
	if !priced {
		// 已完成却没有总价是提供方阶段契约被打破（其 CONTEXT：「已完成必须有可成立的
		// 金额」），不是一种缺席。译成某个出局格会把提供方的缺陷记成这个候选的问题。
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: completed evaluation carries no total",
			ErrUntranslatableEvaluation)
	}
	amount, err := psdomain.NewChannelCostAmount(total.Amount().String())
	if err != nil {
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: cost amount %q: %v",
			ErrUntranslatableEvaluation, total.Amount().String(), err)
	}
	currency, err := psdomain.NewChannelCostCurrency(total.Currency().String())
	if err != nil {
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: cost currency %q: %v",
			ErrUntranslatableEvaluation, total.Currency().String(), err)
	}
	cost, err := psdomain.PricedChannelCandidate(candidate, amount, currency)
	if err != nil {
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: priced channel candidate: %v",
			ErrUntranslatableEvaluation, err)
	}
	return cost, nil
}

func unpriceable(
	candidate psdomain.ChannelCandidateID,
	grade psdomain.ChannelCostUnavailability,
) (psdomain.ChannelCandidateCost, error) {
	cost, err := psdomain.UnpriceableChannelCandidate(candidate, grade)
	if err != nil {
		return psdomain.ChannelCandidateCost{}, fmt.Errorf("%w: unpriceable channel candidate %s: %v",
			ErrUntranslatableEvaluation, grade, err)
	}
	return cost, nil
}
