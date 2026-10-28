package main

import (
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// PP 评价已记录 → FanOut（先 SA 供应商预期成本形成，sa-cc/01、ADR-0200；再 SA 预估客户费用形成，ADR-0190 决定五、六。
// 两路各认一个方向，另一个方向的评价在各自那一路入账走掉）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		buyEvaluations, err := formSupplierExpectedCostOnBuyEvaluationConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		sellEvaluations, err := formEstimatedCustomerChargeOnSellEvaluationConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: sainbox.BuyEvaluationRecordedEventType, position: positionPrimary, consumer: buyEvaluations,
				sentinels: buyEvaluationRecordedUndecidedSentinels, label: "buy evaluation recorded",
			},
			// 两路各认一个方向、互不读对方的结果，先后没有业务含义；偏移只为把次序写明——与主业务锚点同名次装配即拒。
			{
				event: sainbox.BuyEvaluationRecordedEventType, position: positionPrimary + 10, consumer: sellEvaluations,
				sentinels: sellEvaluationRecordedUndecidedSentinels, label: "sell evaluation recorded",
			},
		}, nil
	})
}
