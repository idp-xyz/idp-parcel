package parcelpricing

import (
	"context"
	"fmt"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 读一版已登记价卡此刻还能不能被选中，以及发布时记下的不透明依据。
// 选用看 PriceCardStanding.Selection：截止与退役已经折进那一段，这里不再判一次。依据原样交回，不解释。

type priceCardStandingLoader interface {
	LoadPriceCardStanding(ctx context.Context, tenant ppdomain.TenantID, plan ppdomain.VersionReference) (ppdomain.PriceCardStanding, bool, error)
}

type publicationBasisLookup interface {
	ReadPublicationBasis(ctx context.Context, tenant ppdomain.TenantID, planID, planVersion string) (string, bool, error)
}

// RegisteredPrices 把价卡册上的选用与发布依据译成消费侧要的事实。
type RegisteredPrices struct {
	standings priceCardStandingLoader
	bases     publicationBasisLookup
}

func NewRegisteredPrices(standings priceCardStandingLoader, bases publicationBasisLookup) (*RegisteredPrices, error) {
	if standings == nil || bases == nil {
		return nil, fmt.Errorf("party commercial: registered price reading needs the price card register and the publication basis record")
	}
	return &RegisteredPrices{standings: standings, bases: bases}, nil
}

var _ ports.RegisteredPriceReader = (*RegisteredPrices)(nil)

func (reader *RegisteredPrices) ReadRegisteredPrice(
	ctx context.Context,
	tenant domain.TenantID,
	planID, planVersion string,
	at time.Time,
) (ports.RegisteredPriceFact, error) {
	var none ports.RegisteredPriceFact
	if at.IsZero() || tenant.String() == "" || planID == "" || planVersion == "" {
		return none, fmt.Errorf("party commercial: registered price reading needs a tenant, a plan and the time")
	}
	pricingTenant, err := ppdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("party commercial: registered price tenant: %w", err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactPricingPlan, planID, planVersion)
	if err != nil {
		return none, fmt.Errorf("party commercial: registered price plan: %w", err)
	}
	standing, found, err := reader.standings.LoadPriceCardStanding(ctx, pricingTenant, reference)
	if err != nil {
		return none, err
	}
	if !found {
		return none, nil
	}
	period, selects := standing.Selection()
	basis, hasBasis, err := reader.bases.ReadPublicationBasis(ctx, pricingTenant, planID, planVersion)
	if err != nil {
		return none, err
	}
	return ports.RegisteredPriceFact{
		OnRegister: true, SelectsNow: selects && period.Contains(at),
		PublicationBasis: basis, HasPublicationBasis: hasBasis,
	}, nil
}
