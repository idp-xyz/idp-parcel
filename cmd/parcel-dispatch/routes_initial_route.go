package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// NR 包裹级初始路由判断 → FanOut：VE 投影登在这里，TF 逐段运输准备（ADR-0201 决定七）登在 routes_transport_preparation.go；
// 应消费方里的 NO 今天仍没有消费者，不登。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veInitialRoute, err := deriveInitialRouteConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.InitialRouteFormedEventType, position: positionProjection, consumer: veInitialRoute,
				sentinels: veInitialRouteUndecidedSentinels, label: "initial route projection",
			},
		}, nil
	})
}
