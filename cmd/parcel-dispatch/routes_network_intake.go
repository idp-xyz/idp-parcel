package main

import (
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
)

// PS 有效网络收寄采用结果 → FanOut（先 NR 路由复核 UC-PS-003 步骤 8 → UC-NR-003，再 SA 发起 SELL 评价 ADR-0190）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		intakes, err := networkIntakeConsumer(sources.db, sources.inbox, sources.settings, sources.clock)
		if err != nil {
			return nil, err
		}
		sellOnIntakes, err := requestSellEvaluationOnNetworkIntakeConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 两条链各带各的未决哨兵：合用一个失败码，运维就分不出该去查初始路由那条还是
			// 复核这条等的依赖。复核这条读 PS 取委托接受的时刻（ADR-0279），PS 读不动同样等依赖；
			// 读回的委托与采用记录对不上是不变量破了，不在名单里。
			{
				event: nrinbox.AdoptedNetworkIntakeEventType, position: positionPrimary, consumer: intakes,
				sentinels: []error{nrparcelshipment.ErrReassessmentUndecided, nrparcelshipment.ErrAcceptedParcelUnreadable},
				label:     "reassessment",
			},
			{
				event: nrinbox.AdoptedNetworkIntakeEventType, position: positionBilling, consumer: sellOnIntakes,
				sentinels: sellEvaluationOnNetworkIntakeUndecidedSentinels, label: "sell evaluation on network intake",
			},
		}, nil
	})
}
