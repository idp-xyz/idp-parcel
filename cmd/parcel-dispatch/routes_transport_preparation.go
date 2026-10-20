package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// NR 包裹级初始路由判断 → TF 逐段运输准备（ADR-0201 决定七）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		tfInitialRoute, err := prepareOpportunityOnInitialRouteConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 沿用路由表先 VE 的惯例。FanOut 各路都会调到、各开各的 inbox 事务，次序不影响入账；TF 未决重投时 VE 那一路按 inbox
			// 跳过，不重复投影。
			{
				event: veinbox.InitialRouteFormedEventType, position: positionPrimary, consumer: tfInitialRoute,
				sentinels: tfInitialRoutePreparationUndecidedSentinels, label: "initial route preparation",
			},
		}, nil
	})
}
