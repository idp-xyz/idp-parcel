package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 外部承运轨迹判断 → 只投 VE 投影（label-channel/16）：按 TF CONTEXT 它不构成收寄，PS 面单渠道终局判断不接它。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veExternalTracking, err := deriveExternalTrackingConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.ExternalCarrierTrackingJudgedEventType, position: positionProjection, consumer: veExternalTracking,
				sentinels: veExternalTrackingUndecidedSentinels, label: "external tracking projection",
			},
		}, nil
	})
}
