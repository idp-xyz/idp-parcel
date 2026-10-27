package domain

// candidateSourcePrecedence 是两路来源并存时候选 ETA 的来源次序：运营企业派生的预测在前，承运商提供的在后。
// 对客质量门槛（`PAR-VIS-03`）针对的是运营企业自己形成方法的内部 ETA，承运商的预测是保留来源的外部输入，
// 只在没有运营企业派生的当前版时才顶上。次序是固定的产品口径，不看预测时点也不看可信度：按「谁新谁赢」选会让
// 对客的候选随到达先后来回换手，按「谁能过门槛」选等于替门槛挑结论。
var candidateSourcePrecedence = [...]ETASourceKind{OperatorDerivedETA, CarrierProvidedETA}

// CandidateETAFrom 从同一件包裹、同一里程碑下各来源预测线的当前版里，选出 ETA 客户可见性判断的候选。
// 入参是各线的当前版（每条线至多一版，次序不影响结果）；没有任何当前版时不选。
func CandidateETAFrom(currents []ETAPrediction) (ETAPrediction, bool) {
	for _, source := range candidateSourcePrecedence {
		for _, eta := range currents {
			if eta.source == source {
				return eta, true
			}
		}
	}
	return ETAPrediction{}, false
}
