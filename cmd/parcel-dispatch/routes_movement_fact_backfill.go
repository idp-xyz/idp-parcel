package main

import (
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// 交接信封与揽收信封上移动事实补派生各一路的未决哨兵（ADR-0224）。与同一封信上的投影那一份分开列：补派生还要读对象所在的段、
// 按段列移动事实，又不走信号；交接那份与揽收那份只差回读控制事实那一站。
//
// 不在名单里：ErrHandoverRecordInconsistent、ErrPickupRecordInconsistent 与 ErrMovementFactRecordInconsistent（仓储不变量已破，
// 含按对象列得出、按键却答没有的段，以及按段列出却没声明这一段或装不回的事实）、ErrHandoverUntranslatableAnswer、
// ErrPickupUntranslatableAnswer 与 ErrMovementFactUntranslatableAnswer（引用坏了）、ErrProjectionHandoffPending（要查 outbox
// 下游）、veconsume.ErrUnexpectedProjectionOutcome（封闭集合外）。
var (
	veMovementFactBackfillOnHandoverUndecidedSentinels = []error{
		vetf.ErrHandoverNotVisible,
		vetf.ErrObjectSegmentsNotVisible,
		vetf.ErrMovementFactsOfSegmentNotVisible,
		vetf.ErrProjectionUndecided,
	}
	veMovementFactBackfillOnPickupUndecidedSentinels = []error{
		vetf.ErrPickupNotVisible,
		vetf.ErrObjectSegmentsNotVisible,
		vetf.ErrMovementFactsOfSegmentNotVisible,
		vetf.ErrProjectionUndecided,
	}
)

// TF 权威交接登记、对象级场外揽收登记 → VE 移动事实补派生（票 demo-chain-gaps/53，ADR-0224）。各排在同一封信上的 VE 投影之后、
// 主业务各路之前：补派生与投影同属追踪，不挡在主业务各路后面；投影先落，补派生停在未决重投时，投影那一路按 inbox 跳过。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onHandover, err := backfillMovementFactsOnHandoverConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		onPickup, err := backfillMovementFactsOnPickupConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionProjection + 10, consumer: onHandover,
				sentinels: veMovementFactBackfillOnHandoverUndecidedSentinels, label: "movement fact backfill on handover",
			},
			{
				event: veinbox.OffsitePickupRegisteredEventType, position: positionProjection + 10, consumer: onPickup,
				sentinels: veMovementFactBackfillOnPickupUndecidedSentinels, label: "movement fact backfill on pickup",
			},
		}, nil
	})
}
