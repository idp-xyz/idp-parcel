package main

import (
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// tfDepartureGateReaskUndecidedSentinels 给交接与揽收信封上的补问两路（ADR-0305）。名单只收「此刻读不动、写不进」那一格：控制事实
// 还读不回，段册、移动事实册或补问册此刻读不动，补问交不出去——原样重投即过。
//
// 不收的是重投不会变的那一格（tfinbox.ErrDepartureGateReaskDefect）：某一行装不回、记录与键不符、本上下文自己交出的键立不起来，落
// 发布失败，响给人看。关务那一问读不动不在任何名单里：它是补问读数里的一格（读不动），照样落册入账，由下一份控制事实再问。
var tfDepartureGateReaskUndecidedSentinels = []error{
	tfinbox.ErrDepartureGateReaskUndecided,
}

// veDepartureGateReaskSignalUndecidedSentinels 给 VE 补问信号那一路。不在名单里：ErrMovementFactRecordInconsistent（仓储不变量
// 已破：补问或出发装不回、对不上键、补的不是原版出发、阻断那一格没有关务判断标识）、ErrMovementFactUntranslatableAnswer（引用坏了）、
// ErrDepartureGateSignalHandoffPending（要查 outbox 下游）、veapplication.ErrUnexpectedDepartureGateSignalOutcome（封闭集合外）。
var veDepartureGateReaskSignalUndecidedSentinels = []error{
	vetf.ErrDepartureGateReaskNotVisible,
	vetf.ErrMovementFactNotVisible,
	vetf.ErrDepartureGateSignalUndecided,
}

// TF 权威交接登记、对象级场外揽收登记 → TF 出发门禁补问；TF 出发门禁补问 → VE 出发门禁阻断信号（票 demo-chain-gaps/121，
// ADR-0305）。补问两路排在同一封信上的追踪投影与补派生之后、主业务各路之前：它与补派生同是「先于段登记的出发」的善后，要问关务、
// 比投影慢，停住时不该让追踪那两路等它；主业务各路不必等它。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		onHandover, onPickup, err := departureGateReaskConsumers(sources.db, sources.inbox, sources.outbox, sources.clock)
		if err != nil {
			return nil, err
		}
		signals, err := raiseDepartureGateSignalOnReaskConsumer(sources.db, sources.inbox, sources.projectionDerive)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: veinbox.TransportHandoverRegisteredEventType, position: positionProjection + 20, consumer: onHandover,
				sentinels: tfDepartureGateReaskUndecidedSentinels, label: "departure gate reask on handover",
			},
			{
				event: veinbox.OffsitePickupRegisteredEventType, position: positionProjection + 20, consumer: onPickup,
				sentinels: tfDepartureGateReaskUndecidedSentinels, label: "departure gate reask on pickup",
			},
			{
				event: veinbox.DepartureGateReaskRecordedEventType, position: positionProjection, consumer: signals,
				sentinels: veDepartureGateReaskSignalUndecidedSentinels, label: "departure gate signal on reask",
			},
		}, nil
	})
}
