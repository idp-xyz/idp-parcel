package main

import (
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
)

// 实际测量登记或作废 → 路由在最近一次已判控制触发的位置上按此刻仍有效的实测再判一次。
// 结算重评登在 routes_measurement_change.go，名次是 positionBilling。本路用 positionPrimary，只为同一事件上名次不重复；
// FanOut 每一路都调、各记各的 inbox 账，先后没有业务含义。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := reassessOnMeasurementChangeConsumer(
			sources.db, sources.inbox, sources.settings.purpose, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event:     nrinbox.ActualMeasurementChangedEventType,
			position:  positionPrimary,
			consumer:  consumer,
			sentinels: reassessOnMeasurementChangeUndecidedSentinels,
			label:     "reassess on measurement change",
		}}, nil
	})
}
