package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// PS 包裹服务终局形成 → FanOut（先 VE 投影，再 SA 发起 SELL 评价 ADR-0190，再 SA 给已形成的费用记确认依据 ADR-0336；不投 PS：终局是 PS 自家事实，
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
		confirmOnFinals, err := confirmationBasisOnFinalOutcomeConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
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
			// 与上一路互不读对方的结果：这一路只认已形成的费用，按终局计费的那件费用此刻还没形成，由形成那一笔补记依据。
			{
				event: veinbox.FinalOutcomeFormedEventType, position: positionBilling + 10, consumer: confirmOnFinals,
				sentinels: confirmationBasisOnFinalOutcomeUndecidedSentinels, label: "confirmation basis on final outcome",
			},
		}, nil
	})
}
