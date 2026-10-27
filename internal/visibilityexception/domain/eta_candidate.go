package domain

// candidateSourcePrecedence 是候选 ETA 的来源优先序：运营企业派生的预测在前，承运商提供的在后。对客质量门槛
// （`PAR-VIS-03`）面向运营企业自己有形成方法的预测，承运商的预测是保留来源的外部输入，排在后面作后备。
// 次序是固定的产品口径，只此一处；它不看预测时点：按「谁新谁赢」选会让对客的候选随到达先后来回换手。
var candidateSourcePrecedence = [...]ETASourceKind{OperatorDerivedETA, CarrierProvidedETA}

// CandidateETAFrom 从同一件包裹、同一里程碑下各来源预测线的当前版里，选出 ETA 客户可见性判断的候选：
// 按来源优先序取第一个满足适用规则质量门槛的那一版；没有一个满足，或规则缺失、冲突而无从比对门槛时，取来源优先序
// 最靠前的那一版（质量未达时判断答 ETA 不可见，规则缺失或冲突时答待判断，候选都照实记下）。租户在规则册里把承运商的
// 可信度依据登为达标，就是愿意对客展示它，所以运营企业那一版不达标而承运商那一版达标时，候选是承运商那一版。
// 入参是各线的当前版（每条线至多一版，次序不影响结果），没有任何当前版时不选。结果只由各线当前版与规则决定，
// 与到达先后无关。
func CandidateETAFrom(currents []ETAPrediction, basis ETAVisibilityRuleBasis) (ETAPrediction, bool) {
	var first ETAPrediction
	hasFirst := false
	rule, configured := basis.Rule()
	for _, source := range candidateSourcePrecedence {
		for _, eta := range currents {
			if eta.source != source {
				continue
			}
			if !hasFirst {
				first, hasFirst = eta, true
			}
			if configured && rule.qualifies(eta.confidence) {
				return eta, true
			}
		}
	}
	return first, hasFirst
}
