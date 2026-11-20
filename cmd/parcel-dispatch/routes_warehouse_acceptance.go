package main

import (
	wfinbox "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/inbox"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// 接受决定 → 仓配装箱记录。开关关着时这一路不登记（ADR-0375 决定四）。
// 排在投影和初始路由之后：这一路失败重投时，前面各路按自己的 inbox 跳过。
func init() {
	registerWarehouseFulfillmentRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		waves, err := whpostgres.NewWaves(sources.db)
		if err != nil {
			return nil, err
		}
		recorder := warehouseapp.NewAcceptanceHandler(waves, sources.clock)
		consumer, err := wfinbox.NewAcceptanceDecisionConsumer(sources.db.Transactor(), sources.inbox, recorder)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: wfinbox.AcceptanceDecisionFormedEventType, position: positionLast, consumer: consumer,
			untranslated: true, label: "warehouse acceptance",
		}}, nil
	})
}
