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
			// 去查传输，而实际要查的是消费方等的那个依赖。哨兵只在重投推得动时交出；路由证据
			// 未配置、并列、成本缺依据那几格落`路由未形成`、入账，不经这里。
			//
			// 先 VE 后 NR：NR 腿遇依赖类未决整封回滚重投，反序会把补派生一起堵在重投里。VE 腿
			// 空转或成功不改变 NR 腿的未决记账；VE 腿硬失败把整格升成 publish_failed，那是
			// failureCodeFor 分格的正确行为，不在这里遮。
			{
				event: nrinbox.AcceptedDecisionEventType, position: positionPrimary, consumer: consumer,
				sentinels: []error{nrparcelshipment.ErrRouteHandoffUndecided}, label: "initial route",
			},
		}, nil
	})
}
