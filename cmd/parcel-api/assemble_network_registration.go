package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	networkapp "go.idp.xyz/idp-parcel/internal/networkrouting/application"
)

// transactionalNetworkCatalogRegistration 把七族网络目录登记用例包进一笔事务，理由随
// transactionalPriceCardRegistration：登记册写口按框架合同无事务即拒，事务边界归装配
// 点，形状照登记 CLI 的 execute——在线口与 `parcel-network-register` 消费同一登记用例，
// 答案代数一致（ADR-0085）。
//
// 七族一个包装而不是七个：它们交回同一个 RegisterCatalogResult，传输层也只要一个
// CatalogRegistrar（七族共用，判据在该接口注释）。价卡那两格拆成两个类型是因为两端点
// 各有自己的答案代数，这里不成立。
type transactionalNetworkCatalogRegistration struct {
	transactor bentoapp.Transactor
	inner      *networkapp.NetworkCatalogRegistration
}

var _ networkhttp.CatalogRegistrar = transactionalNetworkCatalogRegistration{}

// withinTransaction 是七个方法共用的事务壳。用例交回业务答案（含幂等重放与治理答案）
// 时提交；返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。
func (registration transactionalNetworkCatalogRegistration) withinTransaction(
	ctx context.Context,
	register func(context.Context) (networkapp.RegisterCatalogResult, error),
) (networkapp.RegisterCatalogResult, error) {
	var result networkapp.RegisterCatalogResult
	err := registration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return networkapp.RegisterCatalogResult{}, err
	}
	return result, nil
}

func (registration transactionalNetworkCatalogRegistration) RegisterNodeVersion(
	ctx context.Context,
	command networkapp.RegisterNodeVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterNodeVersion(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterConnectionVersion(
	ctx context.Context,
	command networkapp.RegisterConnectionVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterConnectionVersion(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterLineVersion(
	ctx context.Context,
	command networkapp.RegisterLineVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterLineVersion(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterServiceAreaVersion(
	ctx context.Context,
	command networkapp.RegisterServiceAreaVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterServiceAreaVersion(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterServiceCalendarVersion(
	ctx context.Context,
	command networkapp.RegisterServiceCalendarVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterServiceCalendarVersion(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterAvailabilityAdjustment(
	ctx context.Context,
	command networkapp.RegisterAvailabilityAdjustmentCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterAvailabilityAdjustment(txCtx, command)
	})
}

func (registration transactionalNetworkCatalogRegistration) RegisterRouteStrategyVersion(
	ctx context.Context,
	command networkapp.RegisterRouteStrategyVersionCommand,
) (networkapp.RegisterCatalogResult, error) {
	return registration.withinTransaction(ctx, func(txCtx context.Context) (networkapp.RegisterCatalogResult, error) {
		return registration.inner.RegisterRouteStrategyVersion(txCtx, command)
	})
}

// buildNetworkCatalogRegistrationOrchestration 装配七个 `/network-catalog-*-registrations`
// 的真编排（ADR-0085，票 admin-write-faces/02 切片 02a）。接真不等墙降：未配置 Intake
// 拒在编排之前，接入渠道就位前编排一次也不会被调到——墙降那笔工作换的只是 Intake。
//
// 目录适配器与查阅口是同一只（NewNetworkCatalog 既是读面也是登记写口，先例在
// `parcel-network-register` 的装配），此处另建一只而不复用 main 里那个变量：查阅参数
// 的类型是读口接口，把它转成写口要断言，那会让「谁能写」在装配点上看不出来。
func buildNetworkCatalogRegistrationOrchestration(db *bentopg.DB) (transactionalNetworkCatalogRegistration, error) {
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return transactionalNetworkCatalogRegistration{}, fmt.Errorf("parcel-api: network catalog registry: %w", err)
	}
	registration, err := networkapp.NewNetworkCatalogRegistration(catalog)
	if err != nil {
		return transactionalNetworkCatalogRegistration{}, fmt.Errorf("parcel-api: network catalog registration: %w", err)
	}
	return transactionalNetworkCatalogRegistration{transactor: db.Transactor(), inner: registration}, nil
}
