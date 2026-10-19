package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnetworkrouting "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// deliveryDispatchOnHandoverUndecidedSentinels 只给「交接 → TF 派送发起」这一路（ADR-0216）。与 VE 交接投影那份分开列：
// 同一封信扇出给两路、各记各的 inbox 账，本路还会停在三条派送要求缝与任务登记册上，合用一份会把 VE 没有的格子也宣布成
// 「等依赖」。
//
// 不在名单里：tfinbox.ErrDeliveryDispatchOnHandoverDefect（本上下文自己交出的键或命令过不了自己的门、装配缺一条缝、执行器
// 答出封闭集合以外的格）、tfapplication.ErrObjectActiveInSeveralSegments（库面不一致）、
// tfapplication.ErrUnexpectedDispatchTaskOutcome——重投都不会变。`要求缺失`不在任何名单里：它入账不重投，不是错误
// （ADR-0216 决定三）。
var deliveryDispatchOnHandoverUndecidedSentinels = []error{
	tfinbox.ErrDeliveryDispatchOnHandoverUndecided,
}

// triggerDeliveryDispatchOnHandoverConsumer 接 TF 权威交接登记 → TF 派送发起（ADR-0216）：信封只带交接键，处理方按键读回
// 交接、找对象此刻在场的段，交既有触发执行器。
//
// 三条派送要求缝照 cmd/parcel-api 的 buildDeliveryDispatchTrigger 另接一份，不跨 main 包复用：两个进程各自是组合根，共用
// 一个装配函数就得把它挪进某个 internal 包，而那里不该知道有几个进程。两份接法要一致——补拍口与本路调的是同一个执行器、
// 读的是同一组所有者，答复才能互相认（同一拍重跑答`已在册`）。
func triggerDeliveryDispatchOnHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	shipmentRequests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery dispatch shipment requests: %w", err)
	}
	places, err := tfparcelshipment.NewDeliveryPlaceSource(shipmentRequests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery place source: %w", err)
	}
	legWindows, err := nrpostgres.NewPlannedLegWindows(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: planned leg windows: %w", err)
	}
	windows, err := tfnetworkrouting.NewDeliveryWindows(legWindows)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery window source: %w", err)
	}
	deliveryConditions, err := pcpostgres.NewDeliveryConditions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery conditions: %w", err)
	}
	conditions, err := tfpartycommercial.NewDeliveryConditionSource(shipmentRequests, deliveryConditions)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery condition source: %w", err)
	}
	return deliveryDispatchOnHandoverConsumerOver(db, inboxStore, clock, places, windows, conditions)
}

// deliveryDispatchOnHandoverConsumerOver 把交接登记册、段登记册与任务登记册接到给定的三条派送要求缝上。三条缝之外的接线
// 只有这一处：生产装配交真适配器，真库用例交替身，两边的交接、段、任务与消费门是同一份接法。
//
// 不另包事务：消费门已在 inbox 那一笔事务里调处理方，任务口落库与 inbox 的已处理同笔提交；同一 ctx 带着同一笔。查在册的
// 读口与任务口落库用同一只任务登记册，理由同 cmd/parcel-api 的 deliveryDispatchTriggerOver。
func deliveryDispatchOnHandoverConsumerOver(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock tfports.Clock,
	places tfports.DeliveryPlaceSource,
	windows tfports.DeliveryWindowSource,
	conditions tfports.DeliveryConditionSource,
) (*tfinbox.TransportHandoverRegisteredConsumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery dispatch transport handovers: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery dispatch fulfillment segments: %w", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery dispatch tasks: %w", err)
	}
	trigger := tfapplication.NewTriggerDeliveryDispatchHandler(tfapplication.TriggerDeliveryDispatchDeps{
		Segments:   segments,
		Tasks:      tasks,
		Places:     places,
		Windows:    windows,
		Conditions: conditions,
		Dispatch:   tfapplication.NewOpenDispatchTaskHandler(tfapplication.OpenDispatchTaskDeps{Tasks: tasks, Clock: clock}),
	})
	handler := tfapplication.NewTriggerDispatchOnHandoverHandler(tfapplication.TriggerDispatchOnHandoverDeps{
		Handovers: handovers,
		Segments:  segments,
		Trigger:   trigger,
		Clock:     clock,
	})
	consumer, err := tfinbox.NewTransportHandoverRegisteredConsumer(db.Transactor(), inboxStore, handler)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery dispatch on handover consumer: %w", err)
	}
	return consumer, nil
}
