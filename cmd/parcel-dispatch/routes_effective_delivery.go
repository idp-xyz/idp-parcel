package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// TF 有效交付登记 → FanOut（先 VE 投影，再 PS 终局 UC-PS-004，再 TF 实际履约发生项自动形成 ADR-0218——那一路登在
// routes_actual_fulfillment.go）。只接 `effective-delivery.registered`，不接 `offsite-pickup.formed`。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		finals, err := adoptEffectiveDeliveryConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		veDelivery, err := deriveDeliveryConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.EffectiveDeliveryRegisteredEventType, position: positionProjection, consumer: veDelivery,
				sentinels: veDeliveryUndecidedSentinels, label: "delivery projection",
			},
			// 终局这条链的未决面是交付可见性、目标委托与终局规则三样——都是「等一个依赖」
			// 而不是发布失败。键/本体不符、交接待发、反查歧义不在这份名单里。
			{
				event: psinbox.EffectiveDeliveryRegisteredEventType, position: positionPrimary, consumer: finals,
				sentinels: effectiveDeliveryUndecidedSentinels, label: "effective delivery",
			},
		}, nil
	})
}
