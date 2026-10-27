package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// VE 内部 ETA 预测新版本 → ETA 客户可见性判断，判断落库后同一事务里重派生客户视图（UC-VE-003、UC-VE-008）。
//
// 只一路、不扇出：预测版本是 VE 自家事实，PS 与 TF 侧今天没有它的消费者——登记接不住的比不登记更糟
// （ADR-0049 决定三「没有订阅者的事件类型显式失败并入账」）。信封由 FormETAHandler 的 ETAHandoff 入队；今天
// 生产上还没有人调 FormETAHandler，所以这一路登了也暂时空转，接上预测来源的那一天它就是现成的下游。
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
