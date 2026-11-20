package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// UnclaimedArrivalWorkFactAuthenticator 以作业事实登记能力面铸待认领到货登记的信封，并判准入范围。
// 能力词是 CapabilityWorkFactRegistration 的字面量，事实类型词是 UNCLAIMED_ARRIVAL（ADR-0384 决定三、决定八）。
type UnclaimedArrivalWorkFactAuthenticator struct {
	minter ReceiptWorkFactMinter
}

var _ warehousehttp.UnclaimedArrivalWorkFactAuthenticator = (*UnclaimedArrivalWorkFactAuthenticator)(nil)

func NewUnclaimedArrivalWorkFactAuthenticator(minter ReceiptWorkFactMinter) (*UnclaimedArrivalWorkFactAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: unclaimed arrival work fact authenticator needs a minter")
	}
	return &UnclaimedArrivalWorkFactAuthenticator{minter: minter}, nil
}

func (authenticator *UnclaimedArrivalWorkFactAuthenticator) AuthenticateUnclaimedArrival(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	var (
		noneTenant   domain.TenantID
		noneOperator domain.ReceiptOperatorID
		noneDevice   domain.ReceiptDeviceID
	)
	admission := identity.WorkFactAdmission(identity.WorkFactUnclaimedArrival)
	if admission == nil {
		return noneTenant, noneOperator, noneDevice, errors.New("warehouse fulfillment accessidentity: unclaimed arrival is not admitted at mint")
	}
	scope, err := identity.NewWorkScope(identity.WorkScopeKind(scopeKind), scopeReference)
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("%w: %v", warehousehttp.ErrMalformedRequest, err)
	}
	envelope, err := authenticator.minter.MintWorkFact(ctx, identity.NewOperatorCredential(bearerToken), identity.WorkFactRequest{
		WorkScope: scope,
		DeviceID:  deviceID,
		Admission: admission,
	})
	if err != nil {
		return noneTenant, noneOperator, noneDevice, answerReceipt(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	subject := envelope.Subject()
	operator, err := domain.NewReceiptOperatorID(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("warehouse fulfillment accessidentity: operator: %w", err)
	}
	device, err := domain.NewReceiptDeviceID(envelope.DeviceID())
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("warehouse fulfillment accessidentity: device: %w", err)
	}
	return tenant, operator, device, nil
}
