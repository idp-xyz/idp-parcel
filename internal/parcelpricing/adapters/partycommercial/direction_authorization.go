// Package partycommercial 是计价对参与方与商业的消费侧适配器：价卡入册前核对它引用的价格方向授权。
// 评价不经过这里。方向从对方指名的商业价格政策读回来，不在计价领域里另造一份。
package partycommercial

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// DirectionAuthorizations 把计价的核对问句译成参与方与商业的只读口，再把答复译回计价的四格。
type DirectionAuthorizations struct {
	view pcports.DirectionAuthorizationView
}

func NewDirectionAuthorizations(view pcports.DirectionAuthorizationView) (*DirectionAuthorizations, error) {
	if view == nil {
		return nil, fmt.Errorf("parcel pricing: direction authorization reader has no party-commercial view")
	}
	return &DirectionAuthorizations{view: view}, nil
}

var _ ports.DirectionAuthorizationReader = (*DirectionAuthorizations)(nil)

func (reader *DirectionAuthorizations) ReadDirectionAuthorization(
	ctx context.Context,
	query ports.DirectionAuthorizationQuery,
) (ports.DirectionAuthorizationAssessment, error) {
	switch query.Direction {
	case domain.PricingDirectionBuy, domain.PricingDirectionSell, domain.PricingDirectionInternal:
	default:
		return 0, fmt.Errorf("read direction authorization: tenant, direction and at are required")
	}
	if query.At.IsZero() || query.Tenant.String() == "" {
		return 0, fmt.Errorf("read direction authorization: tenant, direction and at are required")
	}
	if query.Authorization.Kind() != domain.ArtifactCommercialAuthorization || query.Authorization.ID() == "" || query.Authorization.Version() == "" {
		return ports.DirectionAuthorizationNotPublished, nil
	}
	tenant, err := pcdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return 0, fmt.Errorf("read direction authorization: %w", err)
	}
	objectID, err := pcdomain.NewCommercialObjectID(query.Authorization.ID())
	if err != nil {
		return 0, fmt.Errorf("read direction authorization: %w", err)
	}
	version, err := pcdomain.NewCommercialVersionLabel(query.Authorization.Version())
	if err != nil {
		return 0, fmt.Errorf("read direction authorization: %w", err)
	}
	fact, err := reader.view.ReadDirectionAuthorization(ctx, tenant, objectID, version, query.At)
	if err != nil {
		return 0, err
	}
	switch fact.Standing() {
	case pcdomain.DirectionAuthorizationNotPublished:
		return ports.DirectionAuthorizationNotPublished, nil
	case pcdomain.DirectionAuthorizationNotInForce:
		return ports.DirectionAuthorizationNotInForce, nil
	case pcdomain.DirectionAuthorizationDirectionUnreadable:
		return ports.DirectionAuthorizationDirectionMismatch, nil
	case pcdomain.DirectionAuthorizationAuthorizes:
		if fact.Direction().String() == query.Direction.String() {
			return ports.DirectionAuthorizationMatches, nil
		}
		return ports.DirectionAuthorizationDirectionMismatch, nil
	default:
		return 0, fmt.Errorf("read direction authorization: standing %s", fact.Standing())
	}
}
