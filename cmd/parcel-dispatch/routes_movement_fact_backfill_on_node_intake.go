package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// 节点收寄形成信封上移动事实补派生的未决哨兵（ADR-0299）：收寄读不回、问节点作业曾装它的单元或单元那一刻的成员读不动、对象的段与
// 按段列事实读不动、派生编排未决，都是等依赖。
//
// 不在名单里：ErrFormedIntakeInconsistent 与 ErrConsolidationMembershipInconsistent（节点作业的答复立不成收寄身份或成员，重问同一个
// 结果）、ErrMovementFactRecordInconsistent（仓储不变量已破，含按对象列得出、按键却答没有的段，以及按段列出却没声明这一段或装不回的
// 事实）、ErrMovementFactUntranslatableAnswer（引用坏了）、ErrProjectionHandoffPending（要查 outbox 下游）、
// veconsume.ErrUnexpectedProjectionOutcome（封闭集合外）。
var veMovementFactBackfillOnNodeIntakeUndecidedSentinels = []error{
	vetf.ErrFormedIntakeNotVisible,
	vetf.ErrConsolidationMembershipNotVisible,
	vetf.ErrObjectSegmentsNotVisible,
	vetf.ErrMovementFactsOfSegmentNotVisible,
	vetf.ErrProjectionUndecided,
}

// NO 节点收寄形成 → VE 移动事实补派生（票 docs-consistency/code/74，ADR-0299）。排在同一封信上的 VE 投影之后、PS 来源采用之前：补派生
// 与投影同属追踪，不挡在来源采用后面；投影先落，补派生停在未决重投时，投影那一路按 inbox 跳过。信封与投影、采用共用，载荷不加字段。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onNodeIntake, err := backfillMovementFactsOnNodeIntakeConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: veinbox.NodeIntakeFormedEventType, position: positionProjection + 10, consumer: onNodeIntake,
			sentinels: veMovementFactBackfillOnNodeIntakeUndecidedSentinels, label: "movement fact backfill on node intake",
		}}, nil
	})
}
