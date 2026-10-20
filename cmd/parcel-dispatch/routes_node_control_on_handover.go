package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 权威交接登记 → NO 节点控制转出（ADR-0214 决定一）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		noHandover, err := transferNodeControlOnHandoverConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 不靠它在扇出里的位置：FanOut 每一路都调，各路各记 inbox 账，谁停在未决都不让别路重做。它与派送发起那一路的先后
			// 没有业务含义；偏移只为把次序写明——与主业务锚点同名次装配即拒。
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionPrimary + 10, consumer: noHandover,
				sentinels: nodeControlOnHandoverUndecidedSentinels, label: "node control on handover",
			},
		}, nil
	})
}
