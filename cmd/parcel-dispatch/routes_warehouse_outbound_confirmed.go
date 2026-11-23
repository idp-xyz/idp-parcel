package main

import (
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	vewarehouse "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/warehousefulfillment"
)

// 出库确认只在开关打开时交给全程追踪。开关关着时这一封没有订阅者。
func init() {
	registerWarehouseFulfillmentRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := vewarehouse.NewOutboundConfirmedConsumer(sources.db.Transactor(), sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: vewarehouse.OutboundConfirmedEventType, position: positionProjection, consumer: consumer,
			sentinels: []error{veconsume.ErrProjectionUndecided}, label: "warehouse outbound confirmed",
		}}, nil
	})
}
