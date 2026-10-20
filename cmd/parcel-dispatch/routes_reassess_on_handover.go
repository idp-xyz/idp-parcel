package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 权威交接登记 → NR 路由复核（UC-NR-003，ADR-0215），排在 TF 派送发起之后。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		nrHandover, err := reassessOnHandoverConsumer(sources.db, sources.inbox, sources.settings, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 不靠它在扇出里的位置，理由同节点控制转出那一路；名次只保住此前的先后。
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionPrimary + 20, consumer: nrHandover,
				sentinels: reassessOnHandoverUndecidedSentinels, label: "reassess on handover",
			},
		}, nil
	})
}
