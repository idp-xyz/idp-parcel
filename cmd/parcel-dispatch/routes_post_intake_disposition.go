package main

import (
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

// PS 收寄后服务处置决定 → TF 另起备用或退运旅程（ADR-0332 请求表，UC-TF-007）。节点运营的扣住与按退运装载、网络路由的重新规划与释放今天没有
// 承接模型，不登；它们日后各自在自己的 routes 文件里登同一类信封。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		tfDisposition, err := startJourneyOnPostIntakeDispositionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: tfinbox.PostIntakeDispositionDecidedEventType, position: positionPrimary, consumer: tfDisposition,
				sentinels: postIntakeDispositionJourneyUndecidedSentinels, label: "post-intake disposition journey start",
			},
		}, nil
	})
}
