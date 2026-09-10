package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfnetworkrouting "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalDeliveryDispatchTrigger 把末端派送任务的内部触发执行器包进一笔事务，理由随本目录其余命令包装：
// 派送任务登记册的写口按框架合同无事务即拒，事务边界归装配点。执行器交回答复（含`不是触发事实`、`要求缺失`、
// `未决`）时事务提交——那些答复不写任何东西，提交一笔空事务无害；返回错误时整笔回滚，端点按 ADR-0022 答
// 「没形成答案」。执行器对段登记册只读、不翻交接，所以这笔事务里唯一的写是形成任务那一行。
type transactionalDeliveryDispatchTrigger struct {
	transactor bentoapp.Transactor
	inner      *tfapp.TriggerDeliveryDispatchHandler
}

var _ tfhttp.DeliveryDispatchTriggerer = transactionalDeliveryDispatchTrigger{}

func (trigger transactionalDeliveryDispatchTrigger) Trigger(
	ctx context.Context,
	command tfapp.TriggerDeliveryDispatchCommand,
) (tfapp.TriggerDeliveryDispatchResult, error) {
	var result tfapp.TriggerDeliveryDispatchResult
	err := trigger.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := trigger.inner.Trigger(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.TriggerDeliveryDispatchResult{}, err
	}
	return result, nil
}

// buildDeliveryDispatchTrigger 装配 `/transport-fulfillment-delivery-dispatch-triggers` 背后的真执行器（ADR-0114 决定二；
// 票 tf-segment-lifecycle-closure/12「生产入口」节——第一条派送要求缝的实施票立执行器的生产入口，票 13 / 14 不重立）。
//
// 派送要求三条缝**全部接真**：`Places` 是 adapters/parcelshipment 的真适配器，窄口交 PS 自己的委托仓储（它兼实现 PS 的
// 收件地点引用读口，读的是委托快照本尊，同一只库）；`Windows` 是 adapters/networkrouting 的真适配器（票
// tf-segment-lifecycle-closure/13），窄口交 NR 自己的按计划履约段引用取段窗口的读口（同一只库，读的是判断本体里那一段）；
// `Conditions` 是 adapters/partycommercial 的真适配器（票 tf-segment-lifecycle-closure/14），两只窄口都给——PS 同一只委托
// 仓储（它也兼实现按包裹身份答商业解析回指的读口）与 PC 的交付条件读口（按回指取闭包、读两层声明），缺一只装配期拒
// （判据同 NewCarrierIdentityDirectory）。自此执行器不再有 *_SOURCE_NOT_WIRED 的停点，每条缝的所有者答什么就是什么
// （ADR-0114 决定三：所有者答「没有」任务保持待形成，不填默认）。任务口不包事务：本函数交出的包装已在事务里，同一 ctx
// 带着同一笔（判据同 buildParticipationEnder）。
//
// 谁按拍调这一口、拍频多大，是租户运营节拍的实例半边：端点挂 UnconfiguredIntake{} 起步，未配置即如实拒（ADR-0055）。
func buildDeliveryDispatchTrigger(db *bentopg.DB) (transactionalDeliveryDispatchTrigger, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: dispatch tasks: %w", err)
	}
	shipmentRequests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	places, err := tfparcelshipment.NewDeliveryPlaceSource(shipmentRequests)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery place source: %w", err)
	}
	legWindows, err := nrpostgres.NewPlannedLegWindows(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: planned leg windows: %w", err)
	}
	windows, err := tfnetworkrouting.NewDeliveryWindows(legWindows)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery window source: %w", err)
	}
	deliveryConditions, err := pcpostgres.NewDeliveryConditions(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery conditions: %w", err)
	}
	conditions, err := tfpartycommercial.NewDeliveryConditionSource(shipmentRequests, deliveryConditions)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery condition source: %w", err)
	}
	handler := tfapp.NewTriggerDeliveryDispatchHandler(tfapp.TriggerDeliveryDispatchDeps{
		Segments:   segments,
		Places:     places,
		Windows:    windows,
		Conditions: conditions,
		Dispatch:   tfapp.NewOpenDispatchTaskHandler(tfapp.OpenDispatchTaskDeps{Tasks: tasks, Clock: systemClock{}}),
	})
	return transactionalDeliveryDispatchTrigger{transactor: db.Transactor(), inner: handler}, nil
}
