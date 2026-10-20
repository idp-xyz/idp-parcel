package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 实际履约发生项自动形成（ADR-0218）：交接登记与有效交付登记两种信封各多扇出一路，都殿后。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onDelivery, err := formActualFulfillmentOnDeliveryConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		onHandover, err := formActualFulfillmentOnHandoverConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.EffectiveDeliveryRegisteredEventType, position: positionLast, consumer: onDelivery,
				sentinels: actualFulfillmentOnDeliveryUndecidedSentinels, label: "actual fulfillment on delivery",
			},
			// 位置不影响记账，但不改分区：实际履约发生项那一路停住时同样挡同一分区后面的信封（ADR-0218 越权风险点 8），交付那一张扇出同理。
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionLast, consumer: onHandover,
				sentinels: actualFulfillmentOnHandoverUndecidedSentinels, label: "actual fulfillment on handover",
			},
		}, nil
	})
}
