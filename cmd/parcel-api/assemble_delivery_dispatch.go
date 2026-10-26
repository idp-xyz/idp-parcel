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
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// transactionalDeliveryDispatchTrigger 把末端派送任务的内部触发执行器包进一笔事务，理由随本目录其余命令包装：
// 派送任务登记册与派送待形成册的写口按框架合同无事务即拒，事务边界归装配点。执行器交回答复时事务提交：形成任务那一行与
// 派送待形成册上那一行（要求缺失立或刷新、形成了结、离场作罢、其余答复记下这一次问过）同笔落下（ADR-0230 决定一）；返回错误时
// 整笔回滚，端点按 ADR-0022 答「没形成答案」。执行器对段登记册只读、不翻交接，这笔事务不碰段与交接。
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

// buildDeliveryDispatchTrigger 装配 `/transport-fulfillment-delivery-dispatch-triggers` 背后的真执行器（ADR-0114 决定二）。
// 这一口是补拍：进程内的那一拍由派发进程随交接登记信封逐对象发起（ADR-0216），调的是同一个执行器。派发进程那一路照这里
// 另接一份三条缝（cmd/parcel-dispatch 的 triggerDeliveryDispatchOnHandoverConsumer，两份为何要一致见那里），改这里的缝
// 要同改那一份。
//
// 派送要求三条缝**全部接真**：`Places` 是 adapters/parcelshipment 的真适配器，窄口交 PS 自己的委托仓储（它兼实现 PS 的
// 收件地点引用读口，读的是委托快照本尊，同一只库）；`Windows` 是 adapters/networkrouting 的真适配器（票
// tf-segment-lifecycle-closure/13），窄口交 NR 自己的按计划履约段引用取段窗口的读口（同一只库，读的是判断本体里那一段）；
// `Conditions` 是 adapters/partycommercial 的真适配器（票 tf-segment-lifecycle-closure/14），两只窄口都给——PS 同一只委托
// 仓储（它也兼实现按包裹身份答商业解析回指的读口）与 PC 的交付条件读口（按回指取闭包、读两层声明），缺一只装配期拒
// （判据同 NewCarrierIdentityDirectory）。自此执行器不再有 *_SOURCE_NOT_WIRED 的停点，每条缝的所有者答什么就是什么
// （ADR-0114 决定三：所有者答「没有」任务保持待形成，不填默认）。
//
// 一拍一对象，没有拍频可选（ADR-0216 决定二）。端点挂运营决定「派送重问」的 Intake（ADR-0230 决定七：再发起的人在管理台上，
// 手里没有设备），渠道没配时那一口如实答未配置（ADR-0055）。按册重问调的也是这一只（buildDeliveryDispatchReask）。
func buildDeliveryDispatchTrigger(db *bentopg.DB) (transactionalDeliveryDispatchTrigger, error) {
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
	return deliveryDispatchTriggerOver(db, places, windows, conditions)
}

// deliveryDispatchTriggerOver 把段登记册与任务登记册接到给定的三条派送要求缝上，包进事务交出。三条缝之外的接线只有
// 这一处：生产装配交真适配器，真库回归交替身，两边的段、任务与事务是同一份接法。
//
// 查在册的读口与任务口落库用的是同一只任务登记册：执行器先按本拍的键查在册、查到就不再向所有者取件，读的必须就是
// 任务口写的那张表。任务口不包事务：本函数交出的包装已在事务里，同一 ctx 带着同一笔（判据同 buildParticipationEnder）。派送待
// 形成册同在这一笔里（ADR-0230 决定一）：这一拍答出的要求缺失、形成或作罢与册上那一行一起提交或一起回滚。
func deliveryDispatchTriggerOver(
	db *bentopg.DB,
	places tfports.DeliveryPlaceSource,
	windows tfports.DeliveryWindowSource,
	conditions tfports.DeliveryConditionSource,
) (transactionalDeliveryDispatchTrigger, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: dispatch tasks: %w", err)
	}
	pendings, err := tfpostgres.NewDeliveryDispatchPendings(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery dispatch pendings: %w", err)
	}
	asks, err := tfpostgres.NewDeliveryDispatchAsks(db)
	if err != nil {
		return transactionalDeliveryDispatchTrigger{}, fmt.Errorf("parcel-api: delivery dispatch asks: %w", err)
	}
	handler := tfapp.NewTriggerDeliveryDispatchHandler(tfapp.TriggerDeliveryDispatchDeps{
		Segments:   segments,
		Tasks:      tasks,
		Places:     places,
		Windows:    windows,
		Conditions: conditions,
		Dispatch:   tfapp.NewOpenDispatchTaskHandler(tfapp.OpenDispatchTaskDeps{Tasks: tasks, Clock: systemClock{}}),
		Pending:    pendings,
		Asks:       asks,
	})
	return transactionalDeliveryDispatchTrigger{transactor: db.Transactor(), inner: handler}, nil
}
