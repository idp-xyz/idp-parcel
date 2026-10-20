package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 移动事实已形成 → VE 投影与阻断信号（票 demo-chain-gaps/33，ADR-0205 决定五）。deriveMovementFactConsumer 交回的已是包好
// 哨兵的那一路（veMovementFactUndecidedSentinels），路由条目处不再翻。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veMovementFactRouted, err := deriveMovementFactConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.MovementFactRecordedEventType, position: positionProjection, consumer: veMovementFactRouted,
				untranslated: true, label: "movement fact projection",
			},
		}, nil
	})
}
