package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	whparty "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/partycommercial"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// 仓配履约各册的生产装配（ADR-0375 决定四）。只在部署开关打开时调用：关着时特性的 build 不走到这里，仓配的适配器与编排
// 一只都不构造。

// inTransaction 是各册登记的事务壳：一条命令一笔事务，登记册的保存在没有环境事务时拒绝（框架合同），登记用例自己不开事务；
// 用例读在册修订、核修订连续与落册因此看同一份快照。用例交回业务答案（含重放、冲突与未受理）时提交，返回错误时整笔回滚，
// 端点按 ADR-0022 答「没形成答案」。
func inTransaction[Result any](
	ctx context.Context,
	transactor bentoapp.Transactor,
	run func(txCtx context.Context) (Result, error),
) (Result, error) {
	var result Result
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		produced, runErr := run(txCtx)
		if runErr != nil {
			return runErr
		}
		result = produced
		return nil
	})
	if err != nil {
		var none Result
		return none, err
	}
	return result, nil
}

// transactionalWarehouseRegistration 是仓登记的事务壳。
type transactionalWarehouseRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterWarehouseHandler
}

func (registration transactionalWarehouseRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterWarehouseCommand,
) (warehouseapp.RegisterWarehouseResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterWarehouseResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// transactionalZoneRegistration 是库区登记的事务壳。核「仓已登记」的读与库区的读写在同一个事务里。
type transactionalZoneRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterZoneHandler
}

func (registration transactionalZoneRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterZoneCommand,
) (warehouseapp.RegisterZoneResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterZoneResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// transactionalLocationTypeRegistration 是库位类型登记的事务壳。核「本租户有没有登记过仓」的读与库位类型的读写在同一个事务里。
type transactionalLocationTypeRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterLocationTypeHandler
}

func (registration transactionalLocationTypeRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterLocationTypeCommand,
) (warehouseapp.RegisterLocationTypeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterLocationTypeResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// transactionalLocationRegistration 是库位登记的事务壳。核仓、库区与库位类型的读与库位的读写在同一个事务里。
type transactionalLocationRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterLocationHandler
}

func (registration transactionalLocationRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterLocationCommand,
) (warehouseapp.RegisterLocationResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterLocationResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// transactionalPackagingMaterialRegistration 是包材登记的事务壳。核「本租户有没有登记过仓」的读与包材的读写在同一个事务里。
type transactionalPackagingMaterialRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterPackagingMaterialHandler
}

func (registration transactionalPackagingMaterialRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterPackagingMaterialCommand,
) (warehouseapp.RegisterPackagingMaterialResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterPackagingMaterialResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// warehouseFulfillmentOrchestration 是仓配各册开着时的真编排：各册的登记（带事务壳）与查阅。查阅读已落册的修订，不开事务。
type warehouseFulfillmentOrchestration struct {
	registration transactionalWarehouseRegistration
	queries      *warehouseapp.QueryWarehousesHandler

	zoneRegistration transactionalZoneRegistration
	zoneQueries      *warehouseapp.QueryZonesHandler

	locationTypeRegistration transactionalLocationTypeRegistration
	locationTypeQueries      *warehouseapp.QueryLocationTypesHandler

	locationRegistration transactionalLocationRegistration
	locationQueries      *warehouseapp.QueryLocationsHandler

	packagingMaterialRegistration transactionalPackagingMaterialRegistration
	packagingMaterialQueries      *warehouseapp.QueryPackagingMaterialsHandler

	goodsRegistration transactionalGoodsRegistration
	goodsQueries      *warehouseapp.QueryGoodsHandler

	preAdvices       transactionalPreAdvice
	preAdviceQueries *warehouseapp.QueryPreAdviceHandler
	preAdviceClose   transactionalPreAdviceClosure

	receiptConfirm        transactionalReceiptConfirmation
	discrepancyConclusion transactionalDiscrepancyConclusion

	measurementRecord  transactionalWarehouseMeasurement
	measurementQueries *warehouseapp.QueryWarehouseMeasurementsHandler
}

// transactionalGoodsRegistration 是商品代录的事务壳。核货主客户账户、核「本租户有没有登记过仓」与商品的读写在同一个事务里。
type transactionalGoodsRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterGoodsHandler
}

func (registration transactionalGoodsRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterGoodsCommand,
) (warehouseapp.RegisterGoodsResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterGoodsResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

// transactionalPreAdvice 是入库预报代录的事务壳。核货主、核仓、取号与落册在同一个事务里。
type transactionalPreAdvice struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.AcceptPreAdviceHandler
}

func (registration transactionalPreAdvice) Accept(
	ctx context.Context,
	command warehouseapp.AcceptPreAdviceCommand,
) (warehouseapp.AcceptPreAdviceResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AcceptPreAdviceResult, error) {
		return registration.inner.Accept(txCtx, command)
	})
}

func (registration transactionalPreAdvice) UpdateNotice(
	ctx context.Context,
	command warehouseapp.UpdatePreAdviceNoticeCommand,
) (warehouseapp.AcceptPreAdviceResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AcceptPreAdviceResult, error) {
		return registration.inner.UpdateNotice(txCtx, command)
	})
}

func (registration transactionalPreAdvice) Withdraw(
	ctx context.Context,
	command warehouseapp.WithdrawPreAdviceCommand,
) (warehouseapp.AcceptPreAdviceResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AcceptPreAdviceResult, error) {
		return registration.inner.Withdraw(txCtx, command)
	})
}

// transactionalReceiptConfirmation 是收货确认的事务壳。入账与推进预报状态在同一笔里：任一成立的行落不下，整笔回滚。
// 转到待认领到货时，收货事实与那件到货也落在这一笔里，任一落不下整笔回滚。
type transactionalReceiptConfirmation struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ConfirmReceiptHandler
}

// transactionalPreAdviceClosure 是入库预报完结的事务壳。少收与推进预报状态在同一笔里。
type transactionalPreAdviceClosure struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ClosePreAdviceHandler
}

func (closure transactionalPreAdviceClosure) ClosePreAdvice(
	ctx context.Context,
	command warehouseapp.ClosePreAdviceCommand,
) (warehouseapp.ClosePreAdviceResult, error) {
	return inTransaction(ctx, closure.transactor, func(txCtx context.Context) (warehouseapp.ClosePreAdviceResult, error) {
		return closure.inner.ClosePreAdvice(txCtx, command)
	})
}

func (confirmation transactionalReceiptConfirmation) Confirm(
	ctx context.Context,
	command warehouseapp.ConfirmReceiptCommand,
) (warehouseapp.ConfirmReceiptResult, error) {
	return inTransaction(ctx, confirmation.transactor, func(txCtx context.Context) (warehouseapp.ConfirmReceiptResult, error) {
		return confirmation.inner.Confirm(txCtx, command)
	})
}

// transactionalDiscrepancyConclusion 是收货差异结论的事务壳。货主口与代录口共用这一只，锁与落册在同一笔里。
type transactionalDiscrepancyConclusion struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ConcludeDiscrepancyHandler
}

func (conclusion transactionalDiscrepancyConclusion) Conclude(
	ctx context.Context,
	command warehouseapp.ConcludeDiscrepancyCommand,
) (warehouseapp.ConcludeDiscrepancyResult, error) {
	return inTransaction(ctx, conclusion.transactor, func(txCtx context.Context) (warehouseapp.ConcludeDiscrepancyResult, error) {
		return conclusion.inner.Conclude(txCtx, command)
	})
}

// transactionalWarehouseMeasurement 是仓测登记的事务壳。锁事实身份、核仓与对象、追加落册在同一笔里。
type transactionalWarehouseMeasurement struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RecordWarehouseMeasurementHandler
}

func (measurement transactionalWarehouseMeasurement) Record(
	ctx context.Context,
	command warehouseapp.RecordWarehouseMeasurementCommand,
) (warehouseapp.RecordWarehouseMeasurementResult, error) {
	return inTransaction(ctx, measurement.transactor, func(txCtx context.Context) (warehouseapp.RecordWarehouseMeasurementResult, error) {
		return measurement.inner.Record(txCtx, command)
	})
}

func buildWarehouseFulfillmentOrchestration(db *bentopg.DB, assigner *docnumber.Assigner) (warehouseFulfillmentOrchestration, error) {
	none := warehouseFulfillmentOrchestration{}
	warehouses, err := whpostgres.NewWarehouses(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: warehouse registry: %w", err)
	}
	catalogue, err := whpostgres.NewWarehouseCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: warehouse catalogue: %w", err)
	}
	zones, err := whpostgres.NewZones(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: zone registry: %w", err)
	}
	zoneCatalogue, err := whpostgres.NewZoneCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: zone catalogue: %w", err)
	}
	locationTypes, err := whpostgres.NewLocationTypes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: location type registry: %w", err)
	}
	locationTypeCatalogue, err := whpostgres.NewLocationTypeCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: location type catalogue: %w", err)
	}
	locations, err := whpostgres.NewLocations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: location registry: %w", err)
	}
	locationCatalogue, err := whpostgres.NewLocationCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: location catalogue: %w", err)
	}
	packagingMaterials, err := whpostgres.NewPackagingMaterials(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: packaging material registry: %w", err)
	}
	packagingMaterialCatalogue, err := whpostgres.NewPackagingMaterialCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: packaging material catalogue: %w", err)
	}
	goods, err := whpostgres.NewGoods(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: goods registry: %w", err)
	}
	goodsCatalogue, err := whpostgres.NewGoodsCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: goods catalogue: %w", err)
	}
	accounts, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: owner account lookup: %w", err)
	}
	owners, err := whparty.NewOwnerAccountLookup(accounts)
	if err != nil {
		return none, fmt.Errorf("parcel-api: owner account lookup: %w", err)
	}
	stock, err := whpostgres.NewStockLedger(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: stock ledger: %w", err)
	}
	stockIDs, err := whidentity.NewStockIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: stock identities: %w", err)
	}
	cartons, err := whpostgres.NewCartonLedger(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: carton ledger: %w", err)
	}
	cartonIDs, err := whidentity.NewCartonIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: carton identities: %w", err)
	}
	discrepancies, err := whidentity.NewReceiptDiscrepancyIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: receipt discrepancy identities: %w", err)
	}
	arrivals, err := whpostgres.NewUnclaimedArrivals(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: unclaimed arrivals: %w", err)
	}
	arrivalIDs, err := whidentity.NewUnclaimedArrivalIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: unclaimed arrival identities: %w", err)
	}
	// 记数用例的生产调用方是收货确认。商品登记仍用这本账问「有没有在库库存」，不另建一套账。
	goodsRegistration := transactionalGoodsRegistration{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewRegisterGoodsHandler(goods, warehouses, stock, owners, systemClock{}),
	}
	preAdviceStore, err := whpostgres.NewPreAdvices(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advices: %w", err)
	}
	preAdviceCatalogue, err := whpostgres.NewPreAdviceCatalogue(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advice catalogue: %w", err)
	}
	receipts, err := whpostgres.NewReceipts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: receipt confirmations: %w", err)
	}
	closures, err := whpostgres.NewPreAdviceClosures(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advice closures: %w", err)
	}
	// 缺分配器必须在取号器之前失败。这一判不能放在函数开头：开关开着、库为空时，build 还没构造任何适配器就会返回，
	// 「开着时装不起来」就证不出它真的去构造了适配器。
	if assigner == nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advice numbers need the document number assigner")
	}
	preAdviceNumbers, err := whpostgres.NewPreAdviceNumbers(db, assigner)
	if err != nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advice numbers: %w", err)
	}
	preAdviceIdentities, err := whidentity.NewPreAdviceIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: inbound pre-advice identities: %w", err)
	}
	preAdviceHandler := warehouseapp.NewAcceptPreAdviceHandler(
		preAdviceStore, warehouses, warehouses, goods, preAdviceNumbers, preAdviceIdentities, owners, systemClock{},
	)
	preAdvices := transactionalPreAdvice{transactor: db.Transactor(), inner: preAdviceHandler}
	preAdviceClose := transactionalPreAdviceClosure{
		transactor: db.Transactor(),
		inner: warehouseapp.NewClosePreAdviceHandler(
			closures, preAdviceStore, preAdviceHandler, receipts, goods, warehouses, discrepancies, systemClock{},
		),
	}
	conclusionStore, err := whpostgres.NewDiscrepancyConclusions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: receipt discrepancy conclusions: %w", err)
	}
	discrepancyConclusion := transactionalDiscrepancyConclusion{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewConcludeDiscrepancyHandler(conclusionStore, warehouses, systemClock{}),
	}
	poster := warehouseapp.NewPostStockMovementHandler(
		stock, warehouses, locations, zones, goods, stockIDs, systemClock{},
	)
	receiptConfirm := transactionalReceiptConfirmation{
		transactor: db.Transactor(),
		inner: warehouseapp.NewConfirmReceiptHandler(
			receipts, preAdviceStore, preAdviceHandler, poster,
			cartons, cartonIDs,
			warehouses, locations, zones, goodsCatalogue, goods,
			discrepancies, arrivals, arrivalIDs, systemClock{},
		),
	}
	measurementStore, err := whpostgres.NewWarehouseMeasurements(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: warehouse measurements: %w", err)
	}
	measurementRecord := transactionalWarehouseMeasurement{
		transactor: db.Transactor(),
		inner: warehouseapp.NewRecordWarehouseMeasurementHandler(
			measurementStore, warehouses, goods, cartons, systemClock{},
		),
	}
	return warehouseFulfillmentOrchestration{
		registration: transactionalWarehouseRegistration{
			transactor: db.Transactor(),
			inner:      warehouseapp.NewRegisterWarehouseHandler(warehouses, systemClock{}),
		},
		// 目录与修订史是同一只读适配器上的方法，用例各收一只。
		queries: warehouseapp.NewQueryWarehousesHandler(catalogue, catalogue),
		zoneRegistration: transactionalZoneRegistration{
			transactor: db.Transactor(),
			// 库区登记核「仓已登记」读的是仓登记册本身：同一个事务里的同一只适配器。
			inner: warehouseapp.NewRegisterZoneHandler(zones, warehouses, systemClock{}),
		},
		zoneQueries: warehouseapp.NewQueryZonesHandler(zoneCatalogue, zoneCatalogue),
		locationTypeRegistration: transactionalLocationTypeRegistration{
			transactor: db.Transactor(),
			// 库位类型登记核「本租户有没有登记过仓」读的也是仓登记册本身：同一个事务里的同一只适配器。
			inner: warehouseapp.NewRegisterLocationTypeHandler(locationTypes, warehouses, systemClock{}),
		},
		locationTypeQueries: warehouseapp.NewQueryLocationTypesHandler(locationTypeCatalogue, locationTypeCatalogue),
		locationRegistration: transactionalLocationRegistration{
			transactor: db.Transactor(),
			// 库位登记核仓、库区与库位类型已登记，读的都是各册的登记册本身：同一个事务里的同一批适配器。
			inner: warehouseapp.NewRegisterLocationHandler(locations, warehouses, zones, locationTypes, systemClock{}),
		},
		locationQueries: warehouseapp.NewQueryLocationsHandler(locationCatalogue, locationCatalogue),
		packagingMaterialRegistration: transactionalPackagingMaterialRegistration{
			transactor: db.Transactor(),
			// 包材登记核「本租户有没有登记过仓」读的也是仓登记册本身：同一个事务里的同一只适配器。
			inner: warehouseapp.NewRegisterPackagingMaterialHandler(packagingMaterials, warehouses, systemClock{}),
		},
		packagingMaterialQueries: warehouseapp.NewQueryPackagingMaterialsHandler(packagingMaterialCatalogue, packagingMaterialCatalogue),
		goodsRegistration:        goodsRegistration,
		goodsQueries:             warehouseapp.NewQueryGoodsHandler(goodsCatalogue, goodsCatalogue, goodsCatalogue, measurementStore, systemClock{}),
		preAdvices:               preAdvices,
		preAdviceQueries:         warehouseapp.NewQueryPreAdviceHandler(preAdviceCatalogue, receipts, goods, closures, conclusionStore, arrivals),
		preAdviceClose:           preAdviceClose,
		receiptConfirm:           receiptConfirm,
		discrepancyConclusion:    discrepancyConclusion,
		measurementRecord:        measurementRecord,
		measurementQueries:       warehouseapp.NewQueryWarehouseMeasurementsHandler(goodsCatalogue, measurementStore),
	}, nil
}
