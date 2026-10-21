package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// TF 权威交接登记 → FanOut：VE 投影登在这里，排第一；同一封信上别的路各登在自己的 routes_*.go，先后只看各路的 position。只接
// `transport-handover.registered`，不投 PS：终局只认有效交付。
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
