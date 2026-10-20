package main

import (
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// TF 运输收费发生项已形成 → 只投 SA 发起 BUY 评价（ADR-0208）。消费者可以先于生产方登进路由表（ADR-0049）；没有生产方
// 交出这一类信封时，本路一封也收不到，不是接不住。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		buyOnOccurrences, err := requestBuyEvaluationOnChargeOccurrenceFormedConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: sainbox.ChargeOccurrenceFormedEventType, position: positionBilling, consumer: buyOnOccurrences,
				sentinels: chargeOccurrenceFormedUndecidedSentinels, label: "buy evaluation on charge occurrence formed",
			},
		}, nil
	})
}
