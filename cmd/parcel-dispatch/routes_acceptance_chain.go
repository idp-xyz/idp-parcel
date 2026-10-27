package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// 接受判断链的几扇门（acceptanceChainConsumers）：PS 委托已提交 → 接受判断链、PS 复核已完成 → 同一条接受判断
// 链的续办门（ADR-0086；两扇门同一编排、各记各的 inbox 账）、PC 参数已登记 → 同一条链
// 的第三扇门（ADR-0094 Decision 四：按信封里的租户取`等待运营登记`队列逐份重驱；本进程
// 里唯一一封由 party-commercial 发、parcel-shipment 收的信）、PS 释放拒收已处理 → 同一条链的续办门（ADR-0293 决定四：
// 人处理完释放未受理册上的一笔之后，链拿委托此刻的当前版本再驱一拍）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		chainGates, err := acceptanceChainConsumers(sources.db, sources.outbox, sources.inbox, sources.settings, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.ShipmentRequestSubmittedEventType, position: positionPrimary, consumer: chainGates.submitted,
				sentinels: acceptanceChainUndecidedSentinels, label: "acceptance chain",
			},
			// 续办门与提交门同一份哨兵：两扇门转交同一个编排，未决面完全相同（ADR-0086 第四条
			// ——重跑停在其它未决即回滚重投）。各列一份就会有漂开的那一天。
			{
				event: psinbox.ManualReviewCompletedEventType, position: positionPrimary, consumer: chainGates.reviewCompleted,
				sentinels: acceptanceChainUndecidedSentinels, label: "review resume",
			},
			// 登记续办门同一份哨兵，理由同复核续办门：它一封信驱多份委托，但每份走的仍是同一条链，停住的
			// 那几份汇总成同一个哨兵交回（ADR-0094 Decision 四）。
			{
				event: psinbox.OperatorRegistrationCompletedEventType, position: positionPrimary, consumer: chainGates.operatorRegistration,
				sentinels: acceptanceChainUndecidedSentinels, label: "operator registration resume",
			},
			// 结算账户那一份登记续办门同一份哨兵：同一段重驱代码、同一条链（票 routing-first-cut/19）。
			{
				event: psinbox.SettlementAccountRegisteredEventType, position: positionPrimary, consumer: chainGates.settlementAccountRegistration,
				sentinels: acceptanceChainUndecidedSentinels, label: "settlement account resume",
			},
			{
				event: psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType, position: positionPrimary, consumer: chainGates.estimateAdoptionRegistration,
				sentinels: acceptanceChainUndecidedSentinels, label: "pre-acceptance estimate adoption resume",
			},
			// 受控补充续办门同一份哨兵，理由同复核续办门：同一条链、同一张未决面（ADR-0106 Decision 三）。
			{
				event: psinbox.SubmissionVersionFormedEventType, position: positionPrimary, consumer: chainGates.submissionVersionFormed,
				sentinels: acceptanceChainUndecidedSentinels, label: "submission version formed resume",
			},
			// 释放拒收处理续办门同一份哨兵，理由同复核续办门：同一条链、同一张未决面（ADR-0293 决定四）。
			{
				event: psinbox.ControlReleaseRefusalHandledEventType, position: positionPrimary, consumer: chainGates.controlReleaseRefusalHandled,
				sentinels: acceptanceChainUndecidedSentinels, label: "control release refusal handled resume",
			},
		}, nil
	})
}
