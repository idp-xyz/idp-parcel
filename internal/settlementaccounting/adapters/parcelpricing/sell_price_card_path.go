package parcelpricing

import (
	"context"
	"fmt"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellPriceCardPaths 把计价的在用价卡解析译成结算要的聚合方式。不挑卡：冲突原样交回。
type SellPriceCardPaths struct {
	cards ppports.PriceCardInForceResolver
}

func NewSellPriceCardPaths(cards ppports.PriceCardInForceResolver) (*SellPriceCardPaths, error) {
	if cards == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: price card resolver is nil")
	}
	return &SellPriceCardPaths{cards: cards}, nil
}

func (adapter *SellPriceCardPaths) ResolveSellPriceCardPath(
	ctx context.Context,
	tenant sadomain.TenantID,
	scope sadomain.EstimatePricingScope,
	at time.Time,
	serviceProduct string,
) (saports.SellPriceCardPath, error) {
	none := saports.SellPriceCardPath{}
	ppTenant, err := ppdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, err
	}
	ppScope, err := ppdomain.NewPricingScopeID(scope.String())
	if err != nil {
		return none, err
	}
	var product ppdomain.ServiceProductReference
	if serviceProduct != "" {
		product, err = ppdomain.NewServiceProductReference(serviceProduct)
		if err != nil {
			return none, err
		}
	}
	resolution, err := adapter.cards.ResolveInForce(ctx, ppTenant, ppScope,
		ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, at, product)
	if err != nil {
		return none, err
	}
	switch resolution.Outcome {
	case ppports.PriceCardNotConfigured:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardNotConfigured}, nil
	case ppports.PriceCardApplicabilityConflict:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardConflict}, nil
	case ppports.PriceCardVersionInForce:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardInForce, Aggregation: string(resolution.Plan.Aggregation())}, nil
	default:
		return none, fmt.Errorf("sell price card path: unhandled in-force outcome %d", resolution.Outcome)
	}
}
