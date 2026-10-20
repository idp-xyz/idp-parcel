package main

import (
	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
)

// SA 评价请求已提交 → PP「按评价请求形成评价」（sa-cc/11），BUY 与 SELL 两种请求信封各一路（ADR-0190 决定四）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		evaluationRequests, err := formEvaluationOnEvaluationRequestConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		sellEvaluationRequests, err := formEvaluationOnSellEvaluationRequestConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: ppinbox.EvaluationRequestSubmittedEventType, position: positionPrimary, consumer: evaluationRequests,
				sentinels: evaluationRequestSubmittedUndecidedSentinels, label: "evaluation request submitted",
			},
			{
				event: ppinbox.SellEvaluationRequestSubmittedEventType, position: positionPrimary, consumer: sellEvaluationRequests,
				sentinels: sellEvaluationRequestSubmittedUndecidedSentinels, label: "sell evaluation request submitted",
			},
		}, nil
	})
}
