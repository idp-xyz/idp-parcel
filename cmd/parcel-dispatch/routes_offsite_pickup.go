package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// TF 对象级场外揽收登记 → FanOut（先 VE 投影，再 PS 来源采用）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		pickups, err := adoptOffsitePickupConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		vePickup, err := derivePickupConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.OffsitePickupRegisteredEventType, position: positionProjection, consumer: vePickup,
				sentinels: vePickupUndecidedSentinels, label: "pickup projection",
			},
			// 揽收这条链自带一份哨兵名单：它的未决面是揽收登记可见性、目标委托与资格目录三样，
			// 没有节点侧那个「实物还没识别」，而多出一格不变量破坏必须响亮而不是等。
			{
				event: psinbox.OffsitePickupRegisteredEventType, position: positionPrimary, consumer: pickups,
				sentinels: offsitePickupUndecidedSentinels, label: "offsite pickup",
			},
		}, nil
	})
}
