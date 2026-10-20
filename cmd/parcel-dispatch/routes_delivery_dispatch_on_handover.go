package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 权威交接登记 → TF 派送发起（ADR-0216）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		tfHandover, err := triggerDeliveryDispatchOnHandoverConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 先 VE 后 TF，沿路由表先 VE 的惯例：派送发起停在未决重投时，VE 那一路按 inbox 跳过，不重复投影；反过来也不让投影等派送。
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionPrimary, consumer: tfHandover,
				sentinels: deliveryDispatchOnHandoverUndecidedSentinels, label: "delivery dispatch on handover",
			},
		}, nil
	})
}
