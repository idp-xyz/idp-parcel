package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalCapacityPoolRegistrar 把每次建池包进一笔事务，理由随 transactionalTransportScheduleRegistrar：池册
// 写口按框架合同无事务即拒，事务边界归装配点；读池、核班次在册与落池同笔，编排返回 error 时整笔回滚。
type transactionalCapacityPoolRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.PrepareTransportOpportunityHandler
}

var _ tfhttp.CapacityPoolRegistrar = transactionalCapacityPoolRegistrar{}

func (registrar transactionalCapacityPoolRegistrar) EstablishPool(
	ctx context.Context,
	command tfapp.EstablishPoolCommand,
) (tfapp.OpportunityResult, error) {
	var result tfapp.OpportunityResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.EstablishPool(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.OpportunityResult{}, err
	}
	return result, nil
}

// buildCapacityPoolRegistration 装配 `/transport-fulfillment-capacity-pool-registrations` 背后的真编排（ADR-0201
// 决定一）。缝三条——容量池册、班次册（核班次在册）与时钟——全接真。消耗交接那条缝不接：这一口只建池，预占、
// 释放与消耗是运营决定，各有自己的装配点；类型上交出去的 CapacityPoolRegistrar 也只有建池一法。受控 CLI
// parcel-transport-register 的 capacity-pool 命令装的是同一个编排。
func buildCapacityPoolRegistration(db *bentopg.DB) (tfhttp.CapacityPoolRegistrar, error) {
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: capacity pools: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport schedules: %w", err)
	}
	handler := tfapp.NewPrepareTransportOpportunityHandler(tfapp.PrepareTransportOpportunityDeps{
		Pools:     pools,
		Schedules: schedules,
		Clock:     systemClock{},
	})
	return transactionalCapacityPoolRegistrar{transactor: db.Transactor(), inner: handler}, nil
}
