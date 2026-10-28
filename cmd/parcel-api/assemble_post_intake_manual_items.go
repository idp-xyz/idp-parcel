package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// transactionalPostIntakeManualItemResult 把登记编排包进一笔事务：读到的「已有结果」与写入同在一个快照里，登记不会撞上半个状态。
type transactionalPostIntakeManualItemResult struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.RegisterPostIntakeManualItemResultHandler
}

var _ shipmenthttp.PostIntakeManualItemResultHandler = transactionalPostIntakeManualItemResult{}

func (registration transactionalPostIntakeManualItemResult) Handle(
	ctx context.Context,
	command shipmentapp.RegisterPostIntakeManualItemResultCommand,
) (shipmentapp.RegisterPostIntakeManualItemResultResult, error) {
	var result shipmentapp.RegisterPostIntakeManualItemResultResult
	err := registration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := registration.inner.Handle(txCtx, command)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return shipmentapp.RegisterPostIntakeManualItemResultResult{}, err
	}
	return result, nil
}

// buildPostIntakeManualItemResult 装配 `/post-intake-disposition-manual-item-results` 的真编排（ADR-0341）。
func buildPostIntakeManualItemResult(db *bentopg.DB) (shipmenthttp.PostIntakeManualItemResultHandler, error) {
	items, err := pspostgres.NewPostIntakeManualItems(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: post-intake manual items: %w", err)
	}
	return transactionalPostIntakeManualItemResult{
		transactor: db.Transactor(),
		inner: shipmentapp.NewRegisterPostIntakeManualItemResultHandler(shipmentapp.RegisterPostIntakeManualItemResultDeps{
			ManualItems: items,
			Clock:       systemClock{},
		}),
	}, nil
}
