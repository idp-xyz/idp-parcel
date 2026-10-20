package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 替代/退运旅程启动 → 只投 VE 投影（不 FanOut：一封信带全体成员，消费侧按成员循环拆分派生，ADR-0066）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veExceptionJourney, err := deriveExceptionJourneyConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.ExceptionJourneyRecordedEventType, position: positionProjection, consumer: veExceptionJourney,
				sentinels: veExceptionJourneyUndecidedSentinels, label: "exception journey projection",
			},
		}, nil
	})
}
