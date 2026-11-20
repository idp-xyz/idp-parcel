package main

import (
	wfinbox "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/inbox"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// 资料版本与包裹取消只在开关打开时由仓配接。开关关着时这两类信封本来就没有订阅者。
func init() {
	registerWarehouseFulfillmentRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		waves, err := whpostgres.NewWaves(sources.db)
		if err != nil {
			return nil, err
		}
		requests, err := whpostgres.NewOutboundRequests(sources.db)
		if err != nil {
			return nil, err
		}
		handler := warehouseapp.NewFollowUpHandler(requests, waves, nil, sources.clock)
		sourceData, err := wfinbox.NewSourceDataConsumer(sources.db.Transactor(), sources.inbox, handler)
		if err != nil {
			return nil, err
		}
		cancellation, err := wfinbox.NewCancellationConsumer(sources.db.Transactor(), sources.inbox, handler)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{event: wfinbox.SourceDataFormedEventType, position: positionLast, consumer: sourceData, untranslated: true, label: "warehouse source data"},
			{event: wfinbox.CancellationRecordedEventType, position: positionLast, consumer: cancellation, untranslated: true, label: "warehouse cancellation"},
		}, nil
	})
}
