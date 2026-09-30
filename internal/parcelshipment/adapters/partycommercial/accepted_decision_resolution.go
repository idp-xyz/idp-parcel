package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// acceptedShipmentRequests 是接受时解析回指要用的窄读口。委托仓储的键是来源身份。
type acceptedShipmentRequests interface {
	FindBySourceIdentity(ctx context.Context, identity psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error)
}

// acceptedDecisionResolution 从已接受委托的接受决定上读商业解析（ADR-0159）。
type acceptedDecisionResolution struct {
	requests acceptedShipmentRequests
}

// NewAcceptedDecisionResolution 装配生产用的接受时解析回指。requests 为 nil 是装配缺件。
func NewAcceptedDecisionResolution(requests acceptedShipmentRequests) AcceptanceResolutionSource {
	return acceptedDecisionResolution{requests: requests}
}

func (source acceptedDecisionResolution) ResolutionFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
) (psdomain.CommercialResolutionID, bool, error) {
	if source.requests == nil {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("acceptance resolution: shipment requests are nil")
	}
	if query.Shipment.TenantID().String() == "" || query.Parcel.String() == "" {
		return psdomain.CommercialResolutionID{}, false, nil
	}
	if query.Tenant.String() != "" && query.Shipment.TenantID() != query.Tenant {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("%w: query %q shipment %q",
			ErrAcceptanceResolutionTenantMismatch, query.Tenant, query.Shipment.TenantID())
	}
	request, found, err := source.requests.FindBySourceIdentity(ctx, query.Shipment)
	if err != nil {
		return psdomain.CommercialResolutionID{}, false, fmt.Errorf("acceptance resolution: %w", err)
	}
	if !found {
		return psdomain.CommercialResolutionID{}, false, nil
	}
	return request.CommercialResolutionReferenceFor(query.Parcel)
}
