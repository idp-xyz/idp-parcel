package main

import (
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

// TF 实际履约补形成（ADR-0231 决定一、五）：采用门铃与换版门铃各一路，两类信封今天各只有这一路消费者。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onAdoption, err := catchUpActualFulfillmentOnAdoptionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		onRevision, err := catchUpActualFulfillmentOnRevisionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: tfinbox.SegmentCommissionAdoptionRegisteredEventType, position: positionPrimary, consumer: onAdoption,
				sentinels: actualFulfillmentCatchUpOnAdoptionUndecidedSentinels, label: "actual fulfillment catch-up on adoption",
			},
			{
				event: tfinbox.CommissionAgreementRevisedEventType, position: positionPrimary, consumer: onRevision,
				sentinels: actualFulfillmentCatchUpOnRevisionUndecidedSentinels, label: "actual fulfillment catch-up on revision",
			},
		}, nil
	})
}
