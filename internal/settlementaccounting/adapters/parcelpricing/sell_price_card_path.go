package parcelpricing

import (
	"context"
	"fmt"
	"time"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellPriceCardPaths 把计价的在用价卡解析译成结算要的聚合方式。不挑卡：冲突原样交回。
// probe 只在合同声明了不止一个范围时用：按基础运费表有没有这一票决定停在哪一格。没装时多范围仍停在第一张在用卡上。
type SellPriceCardPaths struct {
	cards ppports.PriceCardInForceResolver
	probe *ppapplication.SellPriceCardProbe
}

func NewSellPriceCardPaths(cards ppports.PriceCardInForceResolver) (*SellPriceCardPaths, error) {
	if cards == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: price card resolver is nil")
	}
	return &SellPriceCardPaths{cards: cards}, nil
}

// UseListProbe 装上多范围探路。生产装配必装；单范围不看它。
func (adapter *SellPriceCardPaths) UseListProbe(probe *ppapplication.SellPriceCardProbe) {
	adapter.probe = probe
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

// ResolveSellPriceCardList 在范围多于一格时探路。没装探路器就停在第一张在用卡上，与只问聚合方式时相同。
func (adapter *SellPriceCardPaths) ResolveSellPriceCardList(
	ctx context.Context,
	tenant sadomain.TenantID,
	shipment sadomain.ShipmentReference,
	members []sadomain.ParcelReference,
	currency sadomain.CurrencyCode,
	scopes []sadomain.EstimatePricingScope,
	at time.Time,
	serviceProduct string,
) (saports.SellPriceCardPath, error) {
	if adapter.probe == nil {
		var last saports.SellPriceCardPath
		for _, scope := range scopes {
			path, err := adapter.ResolveSellPriceCardPath(ctx, tenant, scope, at, serviceProduct)
			if err != nil {
				return saports.SellPriceCardPath{}, err
			}
			switch path.Outcome {
			case saports.SellPriceCardNotConfigured:
				last = path
			case saports.SellPriceCardConflict, saports.SellPriceCardInForce:
				return path, nil
			default:
				return saports.SellPriceCardPath{}, fmt.Errorf("sell price card path: unhandled outcome %d", path.Outcome)
			}
		}
		if last.Outcome == saports.SellPriceCardPathOutcomeInvalid {
			last.Outcome = saports.SellPriceCardNotConfigured
		}
		return last, nil
	}
	ppTenant, err := ppdomain.NewTenantID(tenant.String())
	if err != nil {
		return saports.SellPriceCardPath{}, err
	}
	ppScopes := make([]ppdomain.PricingScopeID, 0, len(scopes))
	for _, scope := range scopes {
		translated, err := ppdomain.NewPricingScopeID(scope.String())
		if err != nil {
			return saports.SellPriceCardPath{}, err
		}
		ppScopes = append(ppScopes, translated)
	}
	ppMembers := make([]string, 0, len(members))
	for _, member := range members {
		ppMembers = append(ppMembers, member.String())
	}
	var ppCurrency ppdomain.Currency
	if currency.String() != "" {
		ppCurrency, err = ppdomain.NewCurrency(currency.String())
		if err != nil {
			return saports.SellPriceCardPath{}, err
		}
	}
	var product ppdomain.ServiceProductReference
	if serviceProduct != "" {
		product, err = ppdomain.NewServiceProductReference(serviceProduct)
		if err != nil {
			return saports.SellPriceCardPath{}, err
		}
	}
	resolution, err := adapter.probe.Resolve(ctx, ppTenant, shipment.String(), ppMembers, ppCurrency, ppScopes, at, product)
	if err != nil {
		return saports.SellPriceCardPath{}, err
	}
	switch resolution.Outcome {
	case ppports.PriceCardNotConfigured:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardNotConfigured}, nil
	case ppports.PriceCardApplicabilityConflict:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardConflict}, nil
	case ppports.PriceCardVersionInForce:
		return saports.SellPriceCardPath{Outcome: saports.SellPriceCardInForce, Aggregation: resolution.Plan.Aggregation().String()}, nil
	default:
		return saports.SellPriceCardPath{}, fmt.Errorf("sell price card path: unhandled in-force outcome %d", resolution.Outcome)
	}
}
