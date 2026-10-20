package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// NO 节点收寄形成 → FanOut（先 VE 投影 UC-VE-002，再 PS 来源采用）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		adoptions, err := adoptNodeIntakeConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		veConsumer, err := deriveProjectionConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.NodeIntakeFormedEventType, position: positionProjection, consumer: veConsumer,
				sentinels: veProjectionUndecidedSentinels, label: "projection",
			},
			// 采用这条链的未决面比接受判断链与初始路由的宽：收寄可见性滞后、目标委托还没落到已接受、资格
			// 目录未配置、实物还没识别，四种都是「等一个依赖」而不是发布失败。
			{
				event: psinbox.NodeIntakeFormedEventType, position: positionPrimary, consumer: adoptions,
				sentinels: nodeIntakeUndecidedSentinels, label: "node intake",
			},
		}, nil
	})
}
