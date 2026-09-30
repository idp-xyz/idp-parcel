package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// acceptedDecisionResolution 从已接受委托的接受决定上读商业解析（ADR-0159）。
type acceptedDecisionResolution struct {
	requests shipmentRequestFinder
}

// NewAcceptedDecisionResolution 装配生产用的接受时解析回指。requests 为 nil 是装配缺件。
func NewAcceptedDecisionResolution(requests shipmentRequestFinder) (AcceptanceResolutionSource, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: shipment requests are nil")
	}
	return acceptedDecisionResolution{requests: requests}, nil
}

func (source acceptedDecisionResolution) ResolutionFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
) (psdomain.CommercialResolutionID, bool, error) {
	if source.requests == nil {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("acceptance resolution: shipment requests are nil")
	}
	if query.Shipment == (psdomain.SourceIdentity{}) || query.Parcel == (psdomain.DeclaredParcelID{}) {
		return psdomain.CommercialResolutionID{}, false, ErrAcceptanceResolutionNotFormed
	}
	if query.Tenant != (psdomain.TenantID{}) && query.Shipment.TenantID() != query.Tenant {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("%w: query %q shipment %q",
			ErrAcceptanceResolutionTenantMismatch, query.Tenant, query.Shipment.TenantID())
	}
	request, found, err := source.requests.FindBySourceIdentity(ctx, query.Shipment)
	if err != nil {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("acceptance resolution: %w", err)
	}
	if !found {
		return psdomain.CommercialResolutionID{}, false, ErrAcceptanceResolutionNotFormed
	}
	resolution, present, err := request.CommercialResolutionReferenceFor(query.Parcel)
	if err != nil {
		return psdomain.CommercialResolutionID{}, false, err
	}
	if !present {
		return psdomain.CommercialResolutionID{}, false, ErrAcceptanceResolutionNotFormed
	}
	return resolution, true, nil
}
