package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

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

// warehouseFulfillmentOrchestration 是仓配各册开着时的真编排：各册的登记（带事务壳）与查阅。查阅读已落册的修订，不开事务。
type warehouseFulfillmentOrchestration struct {
	registration transactionalWarehouseRegistration
	queries      *warehouseapp.QueryWarehousesHandler

	zoneRegistration transactionalZoneRegistration
	zoneQueries      *warehouseapp.QueryZonesHandler
}

func buildWarehouseFulfillmentOrchestration(db *bentopg.DB) (warehouseFulfillmentOrchestration, error) {
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
	}, nil
}
