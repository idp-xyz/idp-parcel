package main

import (
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

// 实际测量登记或作废 → 运输准备在已有申报那一份旁边，按此刻的当前有效实测再判一份。
// 路由复核登在 routes_reassess_on_measurement_change.go，名次是 positionPrimary；结算重评登在 routes_measurement_change.go，
// 名次是 positionBilling。这里用 positionPrimary 加一个偏移，只为同一事件上名次不重复；FanOut 每一路都调、各记各的 inbox 账，
// 先后没有业务含义。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := reprepareOnMeasurementChangeConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event:     tfinbox.ActualMeasurementChangedEventType,
			position:  positionPrimary + 10,
			consumer:  consumer,
			sentinels: tfMeasurementChangeUndecidedSentinels,
			label:     "re-prepare on measurement change",
		}}, nil
	})
}
