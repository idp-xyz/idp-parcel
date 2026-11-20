package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnetworkrouting "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfnodeoperations "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// tfInitialRoutePreparationUndecidedSentinels 只给「初始路由 → TF 运输准备」这一路（ADR-0201 决定七）。与 VE 初始路由投影
// 那份分开列：两路收同一封信、各记各的 inbox 账，未决面也各是各的——本路还会停在申报读口、班次与池的册子、准备结果库上，
// 合用一份会把 VE 没有的格子也宣布成「等依赖」。实测读口答不出也停在 ErrPreparationUndecided 上，不另立一个哨兵。
//
// 不在名单里：ErrInitialRouteRecordInconsistent（提供方仓储不变量已破）、ErrUntranslatableAnswer（计划段说不清自己走哪条
// 连接、键维译不过来，或本包译出的命令被编排拒收——重投不会变）、ErrUnexpectedPreparationOutcome，以及编排原样上抛的
// tfports.ErrDeclaredDemandUntranslatable（申报译不成需求量快照）与 tfports.ErrTransportPreparationRecordUnreadable（库里
// 那一份读不懂）——它们不包 ErrPreparationUndecided，名单认不出它们。回读初始路由时 NR 交的
// nrports.ErrStoredRouteRecordUnreadable（库里那一行重建不出来）由处理适配器保链上抛，也不包 ErrInitialRouteNotVisible。
// `未配置`不在任何名单里：它入账不重投，不是错误（ADR-0094 判据、计划 X6）。
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
	handler, err := prepareOnPlannedLegsHandler(db, clock)
	if err != nil {
		return nil, err
	}
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

// prepareOnPlannedLegsHandler 把申报读口与实测读口接到同一只准备编排上。两个入口共用它：初始路由按申报判完再补实测那一份，
// 实测变化只补实测那一份。实测读口为 nil 时编排会响亮失败，这里不留那条退路。
func prepareOnPlannedLegsHandler(db *bentopg.DB, clock systemClock) (*tfapplication.PrepareOnPlannedLegsHandler, error) {
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
	query, err := currentParcelMeasurementQuery(db)
	if err != nil {
		return nil, err
	}
	measured, err := tfnodeoperations.NewMeasuredDemands(query)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: preparation measured demand: %w", err)
	}
	return tfapplication.NewPrepareOnPlannedLegsHandler(tfapplication.PrepareOnPlannedLegsDeps{
		Demand:       demand,
		Schedules:    schedules,
		Pools:        pools,
		Preparations: preparations,
		Clock:        clock,
		Measured:     measured,
	}), nil
}

// reprepareOnMeasurementChangeConsumer 接实测变化 → 在已有申报准备旁边按当前有效实测再判一份。
func reprepareOnMeasurementChangeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	handler, err := prepareOnPlannedLegsHandler(db, clock)
	if err != nil {
		return nil, err
	}
	associations, err := nopostgres.NewParcelAssociations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: parcel associations for measurement re-preparation: %w", err)
	}
	processing, err := tfnodeoperations.NewRePrepareOnMeasurementChange(associations, handler)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: re-prepare on measurement change: %w", err)
	}
	consumer, err := tfinbox.NewMeasurementChangedConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: re-prepare on measurement change consumer: %w", err)
	}
	return consumer, nil
}

// tfMeasurementChangeUndecidedSentinels 只给「实测变化 → TF 再准备」这一路。与同一封信上路由复核、结算重评的名单分开列。
//
// 不在名单里：实测译不成需求量快照、库里那一份读不懂、命令被编排拒收。这些重投不会变。没有实测、定不下、还没有申报准备、
// 窗口已经结束，都入账，不是错误。
var tfMeasurementChangeUndecidedSentinels = []error{
	tfnodeoperations.ErrRePreparationUndecided,
}
