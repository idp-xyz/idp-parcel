package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// agencyReference 问仓配代办这次许不许，获准时带回授予版本。
type agencyReference interface {
	Reference(ctx context.Context, tenantID, ownerID, reason, evidence string) (string, psports.AuthorizationOutcome, error)
}

// WarehouseWithdrawalAuthorizer 只放行仓配履约以货主名义撤回来源为仓配出库的委托（ADR-0371 决定二）。
// 别的请求方或别的来源不在这条授予下，答未形成，不把它们当成获准。
type WarehouseWithdrawalAuthorizer struct {
	agency agencyReference
}

func NewWarehouseWithdrawalAuthorizer(agency *WarehouseAgencyAuthorizer) (*WarehouseWithdrawalAuthorizer, error) {
	if agency == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: warehouse withdrawal authorizer needs an agency authorizer")
	}
	return &WarehouseWithdrawalAuthorizer{agency: agency}, nil
}

var _ psports.WithdrawalAuthorizer = (*WarehouseWithdrawalAuthorizer)(nil)

func (authorizer *WarehouseWithdrawalAuthorizer) AuthorizeWithdrawal(
	ctx context.Context,
	query psports.WithdrawalAuthorizationQuery,
) (psports.WithdrawalAuthorization, error) {
	if query.Requester.String() != psdomain.WarehouseFulfillmentRequester {
		return psports.WithdrawalAuthorization{}, fmt.Errorf("warehouse withdrawal: requester is not warehouse fulfillment")
	}
	if query.Identity.Source().String() != psdomain.SourceWarehouseOutbound {
		return psports.WithdrawalAuthorization{}, fmt.Errorf("warehouse withdrawal: source is not warehouse outbound")
	}
	reference, outcome, err := authorizer.agency.Reference(
		ctx,
		query.Identity.TenantID().String(),
		query.Identity.CustomerAccountID().String(),
		warehouseOutboundWithdrawalReason,
		"warehouse-cutoff:"+query.Identity.TenantID().String()+"/"+query.ShipmentRequestID.String(),
	)
	if err != nil {
		return psports.WithdrawalAuthorization{}, err
	}
	switch outcome {
	case psports.AuthorizationGranted:
		authority, err := psdomain.NewWithdrawalAuthorityReference(reference)
		if err != nil {
			return psports.WithdrawalAuthorization{}, err
		}
		return psports.WithdrawalAuthorization{Outcome: psports.AuthorizationGranted, Authority: authority}, nil
	case psports.AuthorizationRefused:
		return psports.WithdrawalAuthorization{Outcome: psports.AuthorizationRefused}, nil
	case psports.AuthorizationRulesNotConfigured:
		return psports.WithdrawalAuthorization{Outcome: psports.AuthorizationRulesNotConfigured}, nil
	case psports.AuthorizationApplicabilityConflict:
		return psports.WithdrawalAuthorization{Outcome: psports.AuthorizationApplicabilityConflict}, nil
	default:
		return psports.WithdrawalAuthorization{}, fmt.Errorf("warehouse withdrawal: unknown authorization %d", outcome)
	}
}
