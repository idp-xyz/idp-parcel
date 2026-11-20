package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorUnclaimedClaimAuthenticator 认证待认领到货认领与认领撤销（决定种类 UNCLAIMED_ARRIVAL_CLAIM，准入两格同取字面量）。
type OperatorUnclaimedClaimAuthenticator struct {
	minter Minter
}

var _ warehousehttp.UnclaimedArrivalClaimAuthenticator = (*OperatorUnclaimedClaimAuthenticator)(nil)

func NewOperatorUnclaimedClaimAuthenticator(minter Minter) (*OperatorUnclaimedClaimAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: unclaimed arrival claim authenticator needs a minter")
	}
	return &OperatorUnclaimedClaimAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorUnclaimedClaimAuthenticator) AuthenticateUnclaimedArrivalClaim(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionUnclaimedArrivalClaim)
}

// OperatorUnclaimedDispositionAuthenticator 认证待认领到货处置（决定种类 UNCLAIMED_ARRIVAL_DISPOSITION，准入两格同取字面量）。
type OperatorUnclaimedDispositionAuthenticator struct {
	minter Minter
}

var _ warehousehttp.UnclaimedArrivalDispositionAuthenticator = (*OperatorUnclaimedDispositionAuthenticator)(nil)

func NewOperatorUnclaimedDispositionAuthenticator(minter Minter) (*OperatorUnclaimedDispositionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: unclaimed arrival disposition authenticator needs a minter")
	}
	return &OperatorUnclaimedDispositionAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorUnclaimedDispositionAuthenticator) AuthenticateUnclaimedArrivalDisposition(
	ctx context.Context,
	bearerToken string,
) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionUnclaimedArrivalDisposition)
}

func mintWarehouseDecision(
	ctx context.Context,
	minter Minter,
	bearerToken string,
	kind identity.DecisionKind,
) (warehousehttp.OperatorIdentity, error) {
	envelope, err := minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken), identity.OperatorRequest{
		Face:         identity.CapabilityOperationDecision,
		DecisionKind: kind,
		Admission:    &identity.AdmissionRequirement{Capability: kind.String(), FactKind: kind.String()},
	})
	if err != nil {
		return warehousehttp.OperatorIdentity{}, answerGoods(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	registrant, err := domain.NewRegistrant(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return warehousehttp.OperatorIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: registrant: %w", err)
	}
	return warehousehttp.OperatorIdentity{Tenant: tenant, Registrant: registrant}, nil
}
