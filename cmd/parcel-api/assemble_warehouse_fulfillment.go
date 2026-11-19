package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// 仓配履约仓这一册的生产装配（ADR-0375 决定四）。只在部署开关打开时调用：关着时特性的 build 不走到这里，仓配的适配器与编排
// 一只都不构造。

// transactionalWarehouseRegistration 是仓登记的事务壳：一条命令一笔事务，登记册的保存在没有环境事务时拒绝（框架合同），
// 登记用例自己不开事务；用例读在册修订、核修订连续与落册因此看同一份快照。用例交回业务答案（含重放、冲突与未受理）时提交，
// 返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。
type transactionalWarehouseRegistration struct {
	transactor bentoapp.Transactor
	inner      *warehouseapp.RegisterWarehouseHandler
}

func (registration transactionalWarehouseRegistration) Register(
	ctx context.Context,
	command warehouseapp.RegisterWarehouseCommand,
) (warehouseapp.RegisterWarehouseResult, error) {
	var result warehouseapp.RegisterWarehouseResult
	err := registration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := registration.inner.Register(txCtx, command)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return warehouseapp.RegisterWarehouseResult{}, err
	}
	return result, nil
}

// warehouseFulfillmentOrchestration 是仓这一册开着时的真编排：登记（带事务壳）与查阅。查阅读已落册的修订，不开事务。
type warehouseFulfillmentOrchestration struct {
	registration transactionalWarehouseRegistration
	queries      *warehouseapp.QueryWarehousesHandler
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
	return warehouseFulfillmentOrchestration{
		registration: transactionalWarehouseRegistration{
			transactor: db.Transactor(),
			inner:      warehouseapp.NewRegisterWarehouseHandler(warehouses, systemClock{}),
		},
		// 目录与修订史是同一只读适配器上的方法，用例各收一只。
		queries: warehouseapp.NewQueryWarehousesHandler(catalogue, catalogue),
	}, nil
}
