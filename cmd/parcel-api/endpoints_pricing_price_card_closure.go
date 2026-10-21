package main

import (
	"context"
	"fmt"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingPriceCardClosureWiring struct {
	priceCardClosure pricinghttp.PriceCardClosureRegistrar
}

func init() {
	registerEndpoints(endpointFeature[pricingPriceCardClosureWiring]{
		lines: map[string]endpointLine{
			"/pricing-price-card-successor-cuts": {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-price-card-retirements":    {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (pricingPriceCardClosureWiring, error) {
			closure, err := buildPriceCardClosureOrchestration(sources.db)
			return pricingPriceCardClosureWiring{priceCardClosure: closure}, err
		},
		mount: func(channels endpointChannels, wiring pricingPriceCardClosureWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 价卡收尾两口（ADR-0234）与价卡登记同挂操作者渠道的登记册 Intake：它们改的是按范围选卡时同一方案两版之间选
				// 哪一版，提交方与授予都与价卡登记同一份（ADR-0151 的定族判据）；写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/pricing-price-card-successor-cuts", Handler: pricinghttp.NewCutPriceCardVersionEndpoint(channels.operatorRegistries.pricing, wiring.priceCardClosure)},
				{Pattern: "/pricing-price-card-retirements", Handler: pricinghttp.NewRetirePriceCardVersionEndpoint(channels.operatorRegistries.pricing, wiring.priceCardClosure)},
			}
		},
	})
}

// transactionalPriceCardClosure 把一次收尾包进一笔事务：锁旧版行、读两版、写截止或退役与交「参数已登记」必须在同一笔里，
// 锁才守得住收尾的判据，信封也才与收尾行同生共死（ports.PriceCardClosureRegistry）。编排返回 error 时整笔回滚。
type transactionalPriceCardClosure struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.ClosePriceCardVersionHandler
}

var _ pricinghttp.PriceCardClosureRegistrar = transactionalPriceCardClosure{}

func (closure transactionalPriceCardClosure) Cut(
	ctx context.Context,
	command pricingapp.CutPriceCardVersionCommand,
) (pricingapp.PriceCardClosureResult, error) {
	var result pricingapp.PriceCardClosureResult
	err := closure.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = closure.inner.Cut(txCtx, command)
		return handleErr
	})
	if err != nil {
		return pricingapp.PriceCardClosureResult{}, err
	}
	return result, nil
}

func (closure transactionalPriceCardClosure) RetireAtCoveringSuccessor(
	ctx context.Context,
	command pricingapp.RetirePriceCardVersionCommand,
) (pricingapp.PriceCardClosureResult, error) {
	var result pricingapp.PriceCardClosureResult
	err := closure.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = closure.inner.RetireAtCoveringSuccessor(txCtx, command)
		return handleErr
	})
	if err != nil {
		return pricingapp.PriceCardClosureResult{}, err
	}
	return result, nil
}

func buildPriceCardClosureOrchestration(db *bentopg.DB) (pricinghttp.PriceCardClosureRegistrar, error) {
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: price card catalog for closures: %w", err)
	}
	registered, err := pricingParameterRegisteredHandoff(db)
	if err != nil {
		return nil, err
	}
	handler, err := pricingapp.NewClosePriceCardVersionHandler(pricingapp.ClosePriceCardVersionDeps{
		Registry: cards, Registered: registered, Clock: systemClock{},
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: price card closure: %w", err)
	}
	return transactionalPriceCardClosure{transactor: db.Transactor(), inner: handler}, nil
}
