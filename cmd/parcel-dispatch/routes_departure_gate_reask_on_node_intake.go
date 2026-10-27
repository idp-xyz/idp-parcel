package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// NO 节点收寄形成 → TF 随袋走过出发的成员的出发门禁补问（票 docs-consistency/code/103，ADR-0331）；补问信封到 VE 阻断信号的那一路是
// routes_departure_gate_reask.go 那一条，信封与载荷一字不动。
//
// 排在同一封信上的 VE 投影、PS 来源采用与 VE 移动事实补派生之后：本路与补派生一样先向上找候选单元、再逐层往下验证成员，读量随候选
// 单元数与袋大小增长，还要问关务；一次做不完时会吃光这次投递共用的那一个超时，所以主业务先落地，本路是尽力而为的补问（ADR-0299 决定一、
// 越权风险点 7 同一处耦合：一次投递共用一个超时、一封信共用一份失败预算，整封被放弃后本路没有补偿）。各路是独立的 inbox 账、各自一个
// 事务，本路停在未决不挡同一封信上的别的路。
//
// 未决哨兵用交接与揽收上补问同一份（tfDepartureGateReaskUndecidedSentinels）：收寄读不回、曾装它的单元或成员关系读不动、段册与移动事实册
// 与补问册读不动、补问交不出去，原样重投即过；缺陷一格（tfinbox.ErrDepartureGateReaskDefect）落发布失败。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onNodeIntake, err := departureGateReaskOnNodeIntakeConsumer(sources.db, sources.inbox, sources.outbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: veinbox.NodeIntakeFormedEventType, position: positionPrimary + 20, consumer: onNodeIntake,
			sentinels: tfDepartureGateReaskUndecidedSentinels, label: "departure gate reask on node intake",
		}}, nil
	})
}
