package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 权威交接登记 → FanOut：VE 投影登在这里，排第一；TF 派送发起（ADR-0216）、NO 节点控制转出（ADR-0214）、NR 路由复核
// （UC-NR-003，ADR-0215）与 TF 实际履约发生项（ADR-0218）各登在自己的 routes_*.go。只接 `transport-handover.registered`，
// 不投 PS：终局只认有效交付。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veHandover, err := deriveHandoverConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionProjection, consumer: veHandover,
				sentinels: veHandoverUndecidedSentinels, label: "handover projection",
			},
		}, nil
	})
}
