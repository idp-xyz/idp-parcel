package main

import (
	"context"

	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

func init() {
	registerUnwired(pricingPriceCardClosureWiring{priceCardClosure: unwiredPriceCardClosure{}})
}

// unwiredPriceCardClosure 只给装配测试用，答法同 unwired_orchestration.go 里的各占位。
type unwiredPriceCardClosure struct{}

func (unwiredPriceCardClosure) Cut(
	context.Context,
	pricingapp.CutPriceCardVersionCommand,
) (pricingapp.PriceCardClosureResult, error) {
	return pricingapp.PriceCardClosureResult{}, errOrchestrationNotWired
}

func (unwiredPriceCardClosure) RetireAtCoveringSuccessor(
	context.Context,
	pricingapp.RetirePriceCardVersionCommand,
) (pricingapp.PriceCardClosureResult, error) {
	return pricingapp.PriceCardClosureResult{}, errOrchestrationNotWired
}
