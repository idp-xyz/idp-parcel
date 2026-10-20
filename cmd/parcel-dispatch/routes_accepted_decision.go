package main

import (
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
)

// PS 接受决定 → FanOut（先 VE 客户归属确立补派生 UC-VE-008 AT-VE-169，登在 routes_customer_view.go；再 NR 初始路由 UC-NR-001，
// 登在这里）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		consumer, err := acceptanceConsumer(sources.db, sources.outbox, sources.inbox, sources.settings, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			// 未决哨兵在路由条目处翻译（不在 NR 侧，也不在派发器里）：NR 的
			// ErrRouteHandoffUndecided 不翻，失败码会塌成 dispatch.publish_failed，运维据此
			// 去查传输，而实际要查的是消费方等的那个依赖。
			//
			// 先 VE 后 NR：NR 腿今天必撞路由证据实例墙（ROUTE_EVIDENCE_NOT_CONFIGURED），
			// 反序会把补派生一直堵在墙外。VE 腿空转或成功不改变 NR 腿的未决记账；VE 腿硬失败
			// 把整格升成 publish_failed，那是 failureCodeFor 分格的正确行为，不在这里遮。
			{
				event: nrinbox.AcceptedDecisionEventType, position: positionPrimary, consumer: consumer,
				sentinels: []error{nrparcelshipment.ErrRouteHandoffUndecided}, label: "initial route",
			},
		}, nil
	})
}
