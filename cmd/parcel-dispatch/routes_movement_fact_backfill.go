package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// veMovementFactBackfillUndecidedSentinels 只给交接信封上的移动事实补派生那一路（ADR-0224）。与交接投影、移动事实投影两份分开
// 列：本路比交接投影多三站（对象所在的段、按段列的移动事实、补派生那一刻的派生编排），又不走信号那一站。
//
// 不在名单里：ErrHandoverRecordInconsistent 与 ErrMovementFactRecordInconsistent（仓储不变量已破，含按对象列得出、按键取不回的
// 段，以及按段列出却没声明这一段的事实）、ErrHandoverUntranslatableAnswer 与 ErrMovementFactUntranslatableAnswer（引用坏了）、
// ErrProjectionHandoffPending（要查 outbox 下游）、veconsume.ErrUnexpectedProjectionOutcome（封闭集合外）。
var veMovementFactBackfillUndecidedSentinels = []error{
	vetf.ErrHandoverNotVisible,
	vetf.ErrHandoverSegmentNotVisible,
	vetf.ErrMovementFactsOfSegmentNotVisible,
	vetf.ErrProjectionUndecided,
}

// TF 权威交接登记 → VE 移动事实补派生（票 demo-chain-gaps/53，ADR-0224）。排在交接投影之后、各主业务路之前：补派生与投影同属
// 追踪，不挡在派送发起、节点控制与路由复核后面；交接投影先落，补派生停在未决重投时，投影那一路按 inbox 跳过。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		backfill, err := backfillMovementFactsOnHandoverConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionProjection + 10, consumer: backfill,
				sentinels: veMovementFactBackfillUndecidedSentinels, label: "movement fact backfill on handover",
			},
		}, nil
	})
}
