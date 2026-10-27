package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// VE 内部 ETA 预测新版本 → ETA 客户可见性判断，判断落库后同一事务里重派生客户视图（UC-VE-003、UC-VE-008）。
//
// 只一路、不扇出：预测版本是 VE 自家事实，PS 与 TF 侧没有它的消费者——登记接不住的比不登记更糟
// （ADR-0049 决定三「没有订阅者的事件类型显式失败并入账」）。信封由 FormETAHandler 的 ETAHandoff 入队：
// 谁经 FormETAHandler 形成预测版本，这一路就消费谁入队的信封。路由先在这里登记，是让任何预测来源接入时
// 下游已在位，也免得这个事件类型落进「没有订阅者」的显式失败。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		veETAVisibility, err := judgeETAVisibilityOnPredictionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.ETAPredictionFormedEventType, position: positionProjection, consumer: veETAVisibility,
				sentinels: veETAVisibilityUndecidedSentinels, label: "eta visibility judgement",
			},
		}, nil
	})
}
