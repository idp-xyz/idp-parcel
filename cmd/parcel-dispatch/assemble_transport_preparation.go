package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnetworkrouting "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// tfInitialRoutePreparationUndecidedSentinels 只给「初始路由 → TF 运输准备」这一路（ADR-0201 决定七）。与 VE 初始路由投影
// 那份分开列：两路收同一封信、各记各的 inbox 账，未决面也各是各的——本路还会停在申报读口、班次与池的册子、准备结果库上，
// 合用一份会把 VE 没有的格子也宣布成「等依赖」。
//
// 不在名单里：ErrInitialRouteRecordInconsistent（提供方仓储不变量已破）、ErrUntranslatableAnswer（计划段说不清自己走哪条
// 连接、键维译不过来，或本包译出的命令被编排拒收——重投不会变）、ErrUnexpectedPreparationOutcome，以及编排原样上抛的
// tfports.ErrDeclaredDemandUntranslatable（申报译不成需求量快照）与 tfports.ErrTransportPreparationRecordUnreadable（库里
// 那一份读不懂）——它们不包 ErrPreparationUndecided，名单认不出它们。`未配置`不在任何名单里：它入账不重投，不是错误
// （ADR-0094 判据、计划 X6）。
var tfInitialRoutePreparationUndecidedSentinels = []error{
	tfnetworkrouting.ErrInitialRouteNotVisible,
	tfnetworkrouting.ErrPreparationUndecided,
}

// prepareOpportunityOnInitialRouteConsumer 接 NR 包裹级初始路由判断 → TF 逐段运输准备（ADR-0201 决定七、八）。信封只带判断
// 键，计划段由处理适配器按键回读；需求量经 TF 消费侧适配器读 PS 的申报测量；准备结果只落库、不入队信封——它没有消费者
// （计划 X3），所以本路没有 Outbox。
func prepareOpportunityOnInitialRouteConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	routes, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation initial routes: %w", err)
	}
	shipments, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation shipment requests: %w", err)
	}
	demand, err := tfparcelshipment.NewDeclaredDemand(shipments)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation declared demand: %w", err)
	}
	schedules, err := tfpostgres.NewScheduleCandidates(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation schedule candidates: %w", err)
	}
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation capacity pools: %w", err)
	}
	preparations, err := tfpostgres.NewTransportPreparations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: transport preparations: %w", err)
	}
	handler := tfapplication.NewPrepareOnPlannedLegsHandler(tfapplication.PrepareOnPlannedLegsDeps{
		Demand:       demand,
		Schedules:    schedules,
		Pools:        pools,
		Preparations: preparations,
		Clock:        clock,
	})
	processing, err := tfnetworkrouting.NewPrepareOnInitialRoute(routes, handler)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: prepare on initial route: %w", err)
	}
	consumer, err := tfinbox.NewInitialRouteFormedConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: prepare opportunity on initial route consumer: %w", err)
	}
	return consumer, nil
}
