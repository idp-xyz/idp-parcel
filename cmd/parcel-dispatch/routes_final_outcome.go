package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// PS 包裹服务终局形成 → FanOut（先 VE 投影，再 SA 发起 SELL 评价 ADR-0190；不投 PS：终局是 PS 自家事实，
// 让它经调度器消费自己等于把一份事实记两遍）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veFinalOutcome, err := deriveFinalOutcomeConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		sellOnFinals, err := requestSellEvaluationOnFinalOutcomeConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.FinalOutcomeFormedEventType, position: positionProjection, consumer: veFinalOutcome,
				sentinels: veFinalOutcomeUndecidedSentinels, label: "final outcome projection",
			},
			{
				event: veinbox.FinalOutcomeFormedEventType, position: positionBilling, consumer: sellOnFinals,
				sentinels: sellEvaluationOnFinalOutcomeUndecidedSentinels, label: "sell evaluation on final outcome",
			},
		}, nil
	})
}
