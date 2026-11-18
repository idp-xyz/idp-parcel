package main

import (
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// 实际测量登记或作废 → 结算为关联包裹另起销售评价的下一代，并为载有它的收费发生项另起采购评价的下一代。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := reevaluateSellOnMeasurementChangeConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: sainbox.MeasurementChangedEventType, position: positionBilling, consumer: consumer,
			untranslated: true, label: "measurement change",
		}}, nil
	})
}
