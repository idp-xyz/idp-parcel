package application_test

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// pricingParameterDouble 记下交出的「参数已登记」，可令它失败。
type pricingParameterDouble struct {
	intents []ports.PricingParameterRegisteredIntent
	err     error
}

func (double *pricingParameterDouble) HandOffPricingParameterRegistered(
	_ context.Context, intent ports.PricingParameterRegisteredIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}
