package main

import (
	"context"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	whparcel "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/parcelshipment"
	whparty "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/partycommercial"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	whports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
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

	disposalRuleRegistration transactionalDisposalRuleRegistration
	disposalRuleQueries      *warehouseapp.QueryDisposalRulesHandler
	unclaimedArrivalRegister transactionalUnclaimedArrivalRegistration
	unclaimedArrivalManage   transactionalUnclaimedArrivalManagement
	unclaimedArrivalQueries  *warehouseapp.QueryUnclaimedArrivalsHandler
	stockImportSubmit        transactionalStockImport
	stockImportQueries       *whpostgres.StockImports

	internalJobs            transactionalInternalJob
	internalJobQueries      *warehouseapp.QueryInternalJobsHandler
	instructions            transactionalInstruction
	freezes                 transactionalFreeze
	stockQueries            *warehouseapp.QueryStockHandler
	counts                  transactionalCount
	approvals               transactionalApproval
	adjustments             transactionalAdjustment
	jobDiscrepancies        transactionalJobDiscrepancy
	outbounds               transactionalOutbound
	waves                   transactionalWave
	followUps               transactionalFollowUp
	returns                 transactionalReturn
	agents                  transactionalAgent
	charges                 transactionalCharge
	warehouseAdoptions      warehouseAdoptionBridge
	outboundRequestQueries  *whpostgres.OutboundRequests
	waveQueries             *whpostgres.Waves
	singleUnitMatching      *warehouseapp.SingleUnitMatcher
	chargeOccurrenceQueries *whpostgres.Charges
	workspaceQueries        *whpostgres.WorkspaceReads
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

type transactionalDisposalRuleRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterDisposalRuleHandler
}

func (registration transactionalDisposalRuleRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterDisposalRuleCommand,
) (warehouseapp.RegisterDisposalRuleResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterDisposalRuleResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

type transactionalUnclaimedArrivalRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterUnclaimedArrivalHandler
}

func (registration transactionalUnclaimedArrivalRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterUnclaimedArrivalCommand,
) (warehouseapp.RegisterUnclaimedArrivalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterUnclaimedArrivalResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

type transactionalUnclaimedArrivalManagement struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ManageUnclaimedArrivalHandler
}

func (management transactionalUnclaimedArrivalManagement) Claim(
	ctx context.Context,
	command warehouseapp.ClaimUnclaimedArrivalCommand,
) (warehouseapp.ClaimUnclaimedArrivalResult, error) {
	return inTransaction(ctx, management.transactor, func(txCtx context.Context) (warehouseapp.ClaimUnclaimedArrivalResult, error) {
		return management.inner.Claim(txCtx, command)
	})
}

func (management transactionalUnclaimedArrivalManagement) RevokeClaim(
	ctx context.Context,
	command warehouseapp.RevokeUnclaimedClaimCommand,
) (warehouseapp.RevokeUnclaimedClaimResult, error) {
	return inTransaction(ctx, management.transactor, func(txCtx context.Context) (warehouseapp.RevokeUnclaimedClaimResult, error) {
		return management.inner.RevokeClaim(txCtx, command)
	})
}

func (management transactionalUnclaimedArrivalManagement) Dispose(
	ctx context.Context,
	command warehouseapp.DisposeUnclaimedArrivalCommand,
) (warehouseapp.DisposeUnclaimedArrivalResult, error) {
	return inTransaction(ctx, management.transactor, func(txCtx context.Context) (warehouseapp.DisposeUnclaimedArrivalResult, error) {
		return management.inner.Dispose(txCtx, command)
	})
}

type transactionalStockImport struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.StockImportHandler
}

func (submission transactionalStockImport) Submit(
	ctx context.Context,
	command warehouseapp.SubmitStockImportCommand,
) (warehouseapp.StockImportResult, error) {
	return inTransaction(ctx, submission.transactor, func(txCtx context.Context) (warehouseapp.StockImportResult, error) {
		return submission.inner.Submit(txCtx, command)
	})
}

func (submission transactionalStockImport) Approve(
	ctx context.Context,
	command warehouseapp.PostStockImportCommand,
) (warehouseapp.StockImportResult, error) {
	return inTransaction(ctx, submission.transactor, func(txCtx context.Context) (warehouseapp.StockImportResult, error) {
		return submission.inner.Approve(txCtx, command)
	})
}

func (submission transactionalStockImport) Reject(
	ctx context.Context,
	command warehouseapp.RejectStockImportCommand,
) (warehouseapp.StockImportResult, error) {
	return inTransaction(ctx, submission.transactor, func(txCtx context.Context) (warehouseapp.StockImportResult, error) {
		return submission.inner.Reject(txCtx, command)
	})
}

func buildWarehouseFulfillmentOrchestration(db *bentopg.DB, assigner *docnumber.Assigner, isolated *isolatedWriteAdmission, selfAuthority string) (warehouseFulfillmentOrchestration, error) {
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
	stockImports, err := whpostgres.NewStockImports(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: stock imports: %w", err)
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
	disposalRules, err := whpostgres.NewUnclaimedDisposalRules(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: unclaimed disposal rules: %w", err)
	}
	disposalRuleRegistration := transactionalDisposalRuleRegistration{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewRegisterDisposalRuleHandler(disposalRules, warehouses, systemClock{}),
	}
	unclaimedArrivalRegister := transactionalUnclaimedArrivalRegistration{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewRegisterUnclaimedArrivalHandler(arrivals, arrivalIDs, warehouses, systemClock{}),
	}
	unclaimedArrivalManage := transactionalUnclaimedArrivalManagement{
		transactor: db.Transactor(),
		inner: warehouseapp.NewManageUnclaimedArrivalHandler(
			arrivals, disposalRules, preAdviceStore, warehouses, owners, systemClock{},
		),
	}
	internalJobStore, err := whpostgres.NewInternalJobs(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: internal jobs: %w", err)
	}
	discrepancyIDs, err := whidentity.NewStockDiscrepancyIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: stock discrepancy identities: %w", err)
	}
	instructionIDs, err := whidentity.NewInstructionIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: instruction identities: %w", err)
	}
	instructionStore, err := whpostgres.NewInstructions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: instructions: %w", err)
	}
	freezeStore, err := whpostgres.NewFreezes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: freezes: %w", err)
	}
	countStore, err := whpostgres.NewCounts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: counts: %w", err)
	}
	outboundStore, err := whpostgres.NewOutboundRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbound requests: %w", err)
	}
	outboundNumbers, err := whpostgres.NewOutboundNumbers(db, assigner)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbound numbers: %w", err)
	}
	outboundIDs, err := whidentity.NewOutboundIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbound identities: %w", err)
	}
	waveStore, err := whpostgres.NewWaves(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: waves: %w", err)
	}
	holds := whpostgres.LedgerHolds{
		Freezes: whpostgres.NewFreezeHolds(freezeStore), Counts: countStore, Reservations: outboundStore, Allocations: waveStore,
	}
	freezeIDs, err := whidentity.NewFreezeIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: freeze identities: %w", err)
	}
	stockReads, err := whpostgres.NewStockReads(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: stock reads: %w", err)
	}
	stockQueries := warehouseapp.NewQueryStockHandler(stockReads, warehouses, warehouses, locations, zones, outboundStore, goods)
	countIDs, err := whidentity.NewCountIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: count identities: %w", err)
	}
	counts := transactionalCount{
		transactor: db.Transactor(),
		inner: warehouseapp.NewCountHandler(
			countStore, countIDs, discrepancyIDs, warehouses, locations, zones, goods, holds, stock, systemClock{}, warehouses,
		),
	}
	adjustmentStore, err := whpostgres.NewAdjustments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: adjustments: %w", err)
	}
	adjustments := transactionalAdjustment{
		transactor: db.Transactor(),
		inner: warehouseapp.NewAdjustmentHandler(
			adjustmentStore, countStore, poster, stock, warehouses, locations, zones, goods, owners, holds, systemClock{}, warehouses,
		),
	}
	approvals := transactionalApproval{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewCountApprovalHandler(countStore, countStore, poster, holds, systemClock{}, warehouses).WithPickDifferences(waveStore),
	}
	outbounds := transactionalOutbound{
		transactor: db.Transactor(),
		inner: warehouseapp.NewOutboundHandler(
			outboundStore, outboundIDs, outboundNumbers, stock, warehouses, locations, zones, goods,
			holds, owners, systemClock{}, warehouses,
		).WithShortPicks(waveStore),
	}
	pickTaskIDs, err := whidentity.NewPickTaskIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: pick tasks: %w", err)
	}
	opener, err := buildWarehouseOutboundOpener(db, isolated, selfAuthority, assigner)
	if err != nil {
		return none, err
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbound confirmation outbox: %w", err)
	}
	confirmationHandoff, err := whpostgres.NewOutboxOutboundConfirmationHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, err
	}
	releaser, err := buildWarehouseShipmentReleaser(db)
	if err != nil {
		return none, err
	}
	chargeStore, err := whpostgres.NewCharges(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge occurrences: %w", err)
	}
	chargeIDs, err := whidentity.NewChargeIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge occurrences: %w", err)
	}
	chargeHandoff, err := whpostgres.NewOutboxChargeHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, err
	}
	chargeHandler := warehouseapp.NewChargeHandler(chargeStore, chargeIDs, chargeHandoff, warehouses, systemClock{}).WithReadings(goods, measurementStore)
	counts.inner = counts.inner.WithCharges(chargeHandler).WithPickDifferences(waveStore)
	adjustments.inner = adjustments.inner.WithCharges(chargeHandler)
	receiptConfirm.inner = receiptConfirm.inner.WithCharges(chargeHandler)
	waves := transactionalWave{
		transactor: db.Transactor(),
		inner: warehouseapp.NewWaveHandler(
			waveStore, outboundStore, pickTaskIDs, discrepancyIDs, poster, stock, locations, zones, goods, holds, systemClock{}, warehouses,
		).WithConfirmationHandoff(confirmationHandoff).WithApprovalDuties(countStore).WithShipmentReleaser(releaser).WithCharges(chargeHandler).WithPrefetch(opener, measurementStore, packagingMaterials).WithToteWarehouses(warehouses),
		commissions: warehouseapp.NewCommissionHandler(outboundStore, waveStore, opener, warehouses).WithMeasurementAmender(opener),
	}
	followUps := transactionalFollowUp{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewFollowUpHandler(outboundStore, waveStore, opener, systemClock{}),
	}
	returnStore, err := whpostgres.NewReturns(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: returns: %w", err)
	}
	returnNumbers, err := whpostgres.NewReturnNumbers(db, assigner)
	if err != nil {
		return none, fmt.Errorf("parcel-api: return numbers: %w", err)
	}
	returnIDs, err := whidentity.NewReturnIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: return identities: %w", err)
	}
	agentStore, err := whpostgres.NewAgents(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: agents: %w", err)
	}
	agents := transactionalAgent{
		transactor: db.Transactor(),
		inner:      warehouseapp.NewAgentHandler(agentStore, outboundStore, waveStore, warehouses, opener, systemClock{}).WithCharges(chargeHandler),
	}
	returns := transactionalReturn{
		transactor: db.Transactor(),
		inner: warehouseapp.NewReturnHandler(
			returnStore, returnIDs, returnNumbers, warehouses, locations, zones, warehouses, owners, poster, stock,
			warehouseapp.NewRegisterUnclaimedArrivalHandler(arrivals, arrivalIDs, warehouses, systemClock{}),
			arrivals, systemClock{},
		).WithCharges(chargeHandler),
	}
	charges := transactionalCharge{transactor: db.Transactor(), inner: chargeHandler}
	warehouseAdoptions, err := buildWarehouseAdoptionService(db)
	if err != nil {
		return none, err
	}
	jobDiscrepancies := transactionalJobDiscrepancy{
		transactor: db.Transactor(),
		inner: warehouseapp.NewJobDiscrepancyHandler(
			internalJobStore, countStore, poster, stock, holds, systemClock{}, warehouses,
		).WithAllocationFollow(waveStore, pickTaskIDs, locations, zones).WithCatalogues(goods, cartons, cartonIDs),
	}
	freezes := transactionalFreeze{
		transactor: db.Transactor(),
		inner: warehouseapp.NewFreezeHandler(
			freezeStore, freezeIDs, stock, cartons, warehouses, locations, zones, goods, holds, systemClock{}, warehouses,
		),
	}
	instructions := transactionalInstruction{
		transactor: db.Transactor(),
		inner: warehouseapp.NewInstructionHandler(
			instructionStore, instructionIDs, poster, stock, stock, warehouses, locations, zones, goods, goods,
			owners, holds, systemClock{}, warehouses, instructionStore, cartons, cartonIDs, stockIDs,
		).WithCharges(chargeHandler),
	}
	internalJobs := transactionalInternalJob{
		transactor: db.Transactor(),
		inner: warehouseapp.NewRegisterInternalJobHandler(
			internalJobStore, internalJobStore, poster, stock, stock, cartons, cartonIDs,
			warehouses, locations, zones, goods, holds, discrepancyIDs, systemClock{},
		).WithCharges(chargeHandler),
	}
	workspaceReads, err := whpostgres.NewWorkspaceReads(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: workspace reads: %w", err)
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
		disposalRuleRegistration: disposalRuleRegistration,
		disposalRuleQueries:      warehouseapp.NewQueryDisposalRulesHandler(disposalRules, disposalRules),
		unclaimedArrivalRegister: unclaimedArrivalRegister,
		unclaimedArrivalManage:   unclaimedArrivalManage,
		unclaimedArrivalQueries:  warehouseapp.NewQueryUnclaimedArrivalsHandler(arrivals),
		stockImportSubmit: transactionalStockImport{
			transactor: db.Transactor(),
			inner: warehouseapp.NewStockImportHandler(
				stockImports, owners, warehouses, locations, zones, goods, cartons, cartonIDs, poster, systemClock{},
			).WithApproval(countStore, warehouses),
		},
		stockImportQueries:      stockImports,
		internalJobs:            internalJobs,
		internalJobQueries:      warehouseapp.NewQueryInternalJobsHandler(internalJobStore, internalJobStore),
		instructions:            instructions,
		freezes:                 freezes,
		stockQueries:            stockQueries,
		counts:                  counts,
		approvals:               approvals,
		adjustments:             adjustments,
		jobDiscrepancies:        jobDiscrepancies,
		outbounds:               outbounds,
		waves:                   waves,
		followUps:               followUps,
		returns:                 returns,
		agents:                  agents,
		charges:                 charges,
		warehouseAdoptions:      warehouseAdoptions,
		outboundRequestQueries:  outboundStore,
		waveQueries:             waveStore,
		singleUnitMatching:      warehouseapp.NewSingleUnitMatcher(waveStore, goodsCatalogue, systemClock{}),
		chargeOccurrenceQueries: chargeStore,
		workspaceQueries:        workspaceReads,
	}, nil
}

type transactionalCharge struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ChargeHandler
}

func (registration transactionalCharge) RegisterUnits(ctx context.Context, tenant warehousedomain.TenantID, warehouse warehousedomain.WarehouseCode, weight, volume string) (warehouseapp.ChargeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ChargeResult, error) {
		return registration.inner.RegisterUnits(txCtx, tenant, warehouse, weight, volume)
	})
}

func (registration transactionalCharge) RegisterAging(ctx context.Context, tenant warehousedomain.TenantID, warehouse warehousedomain.WarehouseCode, basis string) (warehouseapp.ChargeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ChargeResult, error) {
		return registration.inner.RegisterAging(txCtx, tenant, warehouse, basis)
	})
}

func (registration transactionalCharge) RegisterProject(ctx context.Context, tenant warehousedomain.TenantID, warehouse warehousedomain.WarehouseCode, name string) (warehouseapp.ChargeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ChargeResult, error) {
		return registration.inner.RegisterProject(txCtx, tenant, warehouse, name)
	})
}

func (registration transactionalCharge) CloseStorageDay(ctx context.Context, tenant warehousedomain.TenantID, owner warehousedomain.OwnerAccountID, warehouse warehousedomain.WarehouseCode, day time.Time) (warehouseapp.ChargeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ChargeResult, error) {
		return registration.inner.CloseStorageDay(txCtx, tenant, owner, warehouse, day)
	})
}

type transactionalReturn struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.ReturnHandler
}

type transactionalAgent struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.AgentHandler
}

func (registration transactionalAgent) RegisterConnector(ctx context.Context, tenant warehousedomain.TenantID, warehouse warehousedomain.WarehouseCode) (warehouseapp.AgentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AgentResult, error) {
		return registration.inner.RegisterConnector(txCtx, tenant, warehouse)
	})
}

func (registration transactionalAgent) RecordStatement(ctx context.Context, tenant warehousedomain.TenantID, statement whports.AgentStatement) (warehouseapp.AgentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AgentResult, error) {
		return registration.inner.RecordStatement(txCtx, tenant, statement)
	})
}

func (registration transactionalAgent) Forward(ctx context.Context, tenant warehousedomain.TenantID, request warehousedomain.OutboundRequestID) (warehouseapp.AgentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AgentResult, error) {
		return registration.inner.Forward(txCtx, tenant, request)
	})
}

func (registration transactionalAgent) RecordResult(ctx context.Context, tenant warehousedomain.TenantID, result whports.AgentResult) (warehouseapp.AgentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AgentResult, error) {
		return registration.inner.RecordResult(txCtx, tenant, result)
	})
}

func (registration transactionalReturn) Accept(ctx context.Context, command warehouseapp.AcceptReturnCommand) (warehouseapp.ReturnResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ReturnResult, error) {
		return registration.inner.Accept(txCtx, command)
	})
}

func (registration transactionalReturn) Receive(ctx context.Context, command warehouseapp.ReceiveReturnCommand) (warehouseapp.ReturnResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ReturnResult, error) {
		return registration.inner.Receive(txCtx, command)
	})
}

func (registration transactionalReturn) Dispose(ctx context.Context, command warehouseapp.DisposeReturnCommand) (warehouseapp.ReturnResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ReturnResult, error) {
		return registration.inner.Dispose(txCtx, command)
	})
}

type transactionalFollowUp struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.FollowUpHandler
}

func (registration transactionalFollowUp) Resubmit(ctx context.Context, command warehouseapp.ResubmitCommand) (warehouseapp.FollowUpResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.FollowUpResult, error) {
		return registration.inner.Resubmit(txCtx, command)
	})
}

// transactionalFreeze 是冻结与解冻的事务壳。判定与落册在同一笔里，并与记数串行。
type transactionalFreeze struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.FreezeHandler
}

func (registration transactionalFreeze) Freeze(ctx context.Context, command warehouseapp.FreezeCommand) (warehouseapp.FreezeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.FreezeResult, error) {
		return registration.inner.Freeze(txCtx, command)
	})
}

func (registration transactionalCount) Create(ctx context.Context, command warehouseapp.CreateCountCommand) (warehouseapp.CountResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CountResult, error) {
		return registration.inner.Create(txCtx, command)
	})
}

func (registration transactionalCount) Start(ctx context.Context, command warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CountResult, error) {
		return registration.inner.Start(txCtx, command)
	})
}

func (registration transactionalCount) Record(ctx context.Context, command warehouseapp.RecordCountCommand) (warehouseapp.CountResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CountResult, error) {
		return registration.inner.Record(txCtx, command)
	})
}

func (registration transactionalCount) Complete(ctx context.Context, command warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CountResult, error) {
		return registration.inner.Complete(txCtx, command)
	})
}

func (registration transactionalCount) Void(ctx context.Context, command warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CountResult, error) {
		return registration.inner.Void(txCtx, command)
	})
}

type transactionalAdjustment struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.AdjustmentHandler
}

func (registration transactionalAdjustment) Submit(ctx context.Context, command warehouseapp.SubmitAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AdjustmentResult, error) {
		return registration.inner.Submit(txCtx, command)
	})
}

func (registration transactionalAdjustment) Approve(ctx context.Context, command warehouseapp.ApproveAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AdjustmentResult, error) {
		return registration.inner.Approve(txCtx, command)
	})
}

func (registration transactionalAdjustment) Reject(ctx context.Context, command warehouseapp.RejectAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.AdjustmentResult, error) {
		return registration.inner.Reject(txCtx, command)
	})
}

type transactionalJobDiscrepancy struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.JobDiscrepancyHandler
}

func (registration transactionalJobDiscrepancy) Approve(ctx context.Context, command warehouseapp.ApproveCountCommand) (warehouseapp.ApprovalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ApprovalResult, error) {
		return registration.inner.Approve(txCtx, command)
	})
}

type transactionalOutbound struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.OutboundHandler
}

func (registration transactionalOutbound) Accept(ctx context.Context, command warehouseapp.AcceptOutboundCommand) (warehouseapp.OutboundResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.OutboundResult, error) {
		return registration.inner.Accept(txCtx, command)
	})
}

func (registration transactionalOutbound) RegisterPolicy(ctx context.Context, command warehouseapp.RegisterShortagePolicyCommand) (warehouseapp.OutboundResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.OutboundResult, error) {
		return registration.inner.RegisterPolicy(txCtx, command)
	})
}

type transactionalWave struct {
	transactor  bentoapp.Transactor
	inner       *warehouseapp.WaveHandler
	commissions *warehouseapp.CommissionHandler
}

func buildWarehouseShipmentReleaser(db *bentopg.DB) (*whparcel.Releaser, error) {
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse withdrawal grants: %w", err)
	}
	keys, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse withdrawal resolution keys: %w", err)
	}
	agency, err := psparty.NewWarehouseAgencyAuthorizer(keys, pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants), systemClock{})
	if err != nil {
		return nil, err
	}
	authorizer, err := psparty.NewWarehouseWithdrawalAuthorizer(agency)
	if err != nil {
		return nil, err
	}
	withdrawal, err := buildActingWithdrawal(db, authorizer)
	if err != nil {
		return nil, err
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse withdrawal requests: %w", err)
	}
	authority, err := buildCancellationAuthority(db)
	if err != nil {
		return nil, err
	}
	canceller, err := buildCancelParcelHandler(db, authority)
	if err != nil {
		return nil, err
	}
	return whparcel.NewReleaser(agency, requests, withdrawal, canceller, systemClock{})
}

func buildWarehouseOutboundOpener(db *bentopg.DB, isolated *isolatedWriteAdmission, selfAuthority string, assigner *docnumber.Assigner) (*whparcel.Opener, error) {
	submission, err := buildSubmissionOrchestration(db, isolated, selfAuthority, assigner)
	if err != nil {
		return nil, err
	}
	ownership, err := buildProductionOwnership(db, isolated, selfAuthority)
	if err != nil {
		return nil, err
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse agency grants: %w", err)
	}
	keys, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse agency resolution keys: %w", err)
	}
	authorizer, err := psparty.NewWarehouseAgencyAuthorizer(keys, pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants), systemClock{})
	if err != nil {
		return nil, err
	}
	opener, err := whparcel.NewOpener(authorizer, submission, ownership, systemClock{})
	if err != nil {
		return nil, err
	}
	sourceSubmissions, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse supplement sources: %w", err)
	}
	shipmentRequests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse supplement requests: %w", err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse supplement identities: %w", err)
	}
	amender, err := buildInnerCustomerAmendmentOrchestration(db)
	if err != nil {
		return nil, err
	}
	return opener.WithVersionFormer(psapplication.NewFormNewSubmissionVersionHandler(psapplication.FormNewSubmissionVersionDeps{
		Sources: sourceSubmissions, Requests: shipmentRequests, Identities: identities, Clock: systemClock{},
	})).WithSourceDataAmender(amender), nil
}

func (registration transactionalWave) Release(ctx context.Context, command warehouseapp.ReleaseWaveCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Release(txCtx, command)
	})
}

func (registration transactionalWave) Withdraw(ctx context.Context, command warehouseapp.WithdrawWaveCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Withdraw(txCtx, command)
	})
}

func (registration transactionalWave) RegisterTotes(ctx context.Context, command warehouseapp.RegisterTotesCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.RegisterTotes(txCtx, command)
	})
}

func (registration transactionalWave) RetireTote(ctx context.Context, command warehouseapp.RetireToteCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.RetireTote(txCtx, command)
	})
}

func (registration transactionalWave) RegisterToteRequirement(ctx context.Context, command warehouseapp.RegisterToteRequirementCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.RegisterToteRequirement(txCtx, command)
	})
}

func (registration transactionalWave) ReleaseTote(ctx context.Context, command warehouseapp.ReleaseToteCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.ReleaseTote(txCtx, command)
	})
}

func (registration transactionalWave) ShipPicked(ctx context.Context, command warehouseapp.ShipPickedCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.ShipPicked(txCtx, command)
	})
}

func (registration transactionalWave) Adopt(ctx context.Context, command warehouseapp.AdoptStrategyCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Adopt(txCtx, command)
	})
}

func (registration transactionalWave) Pick(ctx context.Context, command warehouseapp.PickCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Pick(txCtx, command)
	})
}

func (registration transactionalWave) RegisterWaveRule(ctx context.Context, command warehouseapp.RegisterWaveRuleCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.RegisterWaveRule(txCtx, command)
	})
}

func (registration transactionalWave) RunWaveRule(ctx context.Context, command warehouseapp.RunWaveRuleCommand) (warehouseapp.WaveRuleRunResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveRuleRunResult, error) {
		return registration.inner.RunWaveRule(txCtx, command)
	})
}

func (registration transactionalWave) Sort(ctx context.Context, command warehouseapp.SortCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Sort(txCtx, command)
	})
}

func (registration transactionalWave) Check(ctx context.Context, command warehouseapp.CheckCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Check(txCtx, command)
	})
}

func (registration transactionalWave) Return(ctx context.Context, command warehouseapp.ReturnCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Return(txCtx, command)
	})
}

func (registration transactionalWave) Cutoff(ctx context.Context, command warehouseapp.CutoffCommand) (warehouseapp.CutoffResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CutoffResult, error) {
		return registration.inner.Cutoff(txCtx, command)
	})
}

func (registration transactionalWave) Correct(ctx context.Context, command warehouseapp.CorrectConfirmationCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Correct(txCtx, command)
	})
}

func (registration transactionalWave) ApproveReplenishment(ctx context.Context, command warehouseapp.ApproveReplenishmentCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.ApproveReplenishment(txCtx, command)
	})
}

func (registration transactionalWave) Confirm(ctx context.Context, command warehouseapp.ConfirmCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Confirm(txCtx, command)
	})
}

func (registration transactionalWave) RegisterOwnerLabel(ctx context.Context, command warehouseapp.RegisterOwnerLabelCommand) (warehouseapp.WaveResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.RegisterOwnerLabel(txCtx, command)
	})
}

func (registration transactionalWave) Pack(ctx context.Context, command warehouseapp.PackCommand) (warehouseapp.WaveResult, error) {
	packed, err := inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.WaveResult, error) {
		return registration.inner.Pack(txCtx, command)
	})
	if err != nil || registration.commissions == nil {
		return packed, err
	}
	if packed.Outcome() != warehouseapp.WavePacked && packed.Outcome() != warehouseapp.WaveExisting {
		return packed, nil
	}
	// 委托提交另开一笔。打包已经落定；这一笔没提交成，重放打包口会接着提交。
	commissioned, err := inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.CommissionResult, error) {
		return registration.commissions.Submit(txCtx, command.Tenant, command.Request)
	})
	if err != nil {
		return warehouseapp.WaveResult{}, err
	}
	return packed.AfterCommission(commissioned), nil
}

func (registration transactionalOutbound) RecordConsent(ctx context.Context, command warehouseapp.RecordPartialShipmentConsentCommand) (warehouseapp.OutboundResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.OutboundResult, error) {
		return registration.inner.RecordConsent(txCtx, command)
	})
}

func (registration transactionalOutbound) Split(ctx context.Context, command warehouseapp.SplitOutboundCommand) (warehouseapp.SplitResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.SplitResult, error) {
		return registration.inner.Split(txCtx, command)
	})
}

func (registration transactionalOutbound) SplitShortage(ctx context.Context, command warehouseapp.SplitShortageCommand) (warehouseapp.SplitResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.SplitResult, error) {
		return registration.inner.SplitShortage(txCtx, command)
	})
}

func (registration transactionalJobDiscrepancy) Reject(ctx context.Context, command warehouseapp.RejectCountCommand) (warehouseapp.ApprovalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ApprovalResult, error) {
		return registration.inner.Reject(txCtx, command)
	})
}

type transactionalApproval struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.CountApprovalHandler
}

func (registration transactionalApproval) Approve(ctx context.Context, command warehouseapp.ApproveCountCommand) (warehouseapp.ApprovalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ApprovalResult, error) {
		return registration.inner.Approve(txCtx, command)
	})
}

func (registration transactionalApproval) Reject(ctx context.Context, command warehouseapp.RejectCountCommand) (warehouseapp.ApprovalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ApprovalResult, error) {
		return registration.inner.Reject(txCtx, command)
	})
}

func (registration transactionalApproval) RegisterDuty(ctx context.Context, command warehouseapp.RegisterApprovalDutyCommand) (warehouseapp.ApprovalResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.ApprovalResult, error) {
		return registration.inner.RegisterDuty(txCtx, command)
	})
}

type transactionalCount struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.CountHandler
}

func (registration transactionalFreeze) Unfreeze(ctx context.Context, command warehouseapp.UnfreezeCommand) (warehouseapp.FreezeResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.FreezeResult, error) {
		return registration.inner.Unfreeze(txCtx, command)
	})
}

// transactionalInstruction 是加工、换标与开箱的事务壳。
type transactionalInstruction struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.InstructionHandler
}

func (registration transactionalInstruction) Execute(ctx context.Context, command warehouseapp.ExecuteInstructionCommand) (warehouseapp.InstructionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.InstructionResult, error) {
		return registration.inner.Execute(txCtx, command)
	})
}

func (registration transactionalInstruction) Unbox(ctx context.Context, command warehouseapp.UnboxCommand) (warehouseapp.InstructionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.InstructionResult, error) {
		return registration.inner.Unbox(txCtx, command)
	})
}

func (registration transactionalInstruction) Accept(ctx context.Context, command warehouseapp.AcceptInstructionCommand) (warehouseapp.InstructionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.InstructionResult, error) {
		return registration.inner.Accept(txCtx, command)
	})
}

func (registration transactionalInstruction) Withdraw(ctx context.Context, tenant warehousedomain.TenantID, owner warehousedomain.OwnerAccountID, id warehousedomain.InstructionID, key warehousedomain.RequestKey, allowed []warehousedomain.InstructionKind) (warehouseapp.InstructionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.InstructionResult, error) {
		return registration.inner.Withdraw(txCtx, tenant, owner, id, key, allowed)
	})
}

func (registration transactionalInstruction) Void(ctx context.Context, tenant warehousedomain.TenantID, id warehousedomain.InstructionID, operator warehousedomain.Registrant, reason string, allowed []warehousedomain.InstructionKind) (warehouseapp.InstructionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.InstructionResult, error) {
		return registration.inner.Void(txCtx, tenant, id, operator, reason, allowed)
	})
}

// transactionalInternalJob 是库内作业与次品转良品的事务壳。事实、流水与库存差异在同一笔里。
type transactionalInternalJob struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterInternalJobHandler
}

func (registration transactionalInternalJob) Register(
	ctx context.Context,
	command warehouseapp.RegisterInternalJobCommand,
) (warehouseapp.RegisterInternalJobResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RegisterInternalJobResult, error) {
		return registration.inner.Register(txCtx, command)
	})
}

func (registration transactionalInternalJob) Restore(
	ctx context.Context,
	command warehouseapp.RestoreConditionCommand,
) (warehouseapp.RestoreConditionResult, error) {
	return inTransaction(ctx, registration.transactor, func(txCtx context.Context) (warehouseapp.RestoreConditionResult, error) {
		return registration.inner.Restore(txCtx, command)
	})
}
