package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// WarehouseMeasurementAuthenticator 以作业事实登记能力面铸仓测登记的信封，并判这一种事实的准入范围。
// 能力词是 CapabilityWorkFactRegistration 的字面量，事实类型词是 WAREHOUSE_MEASUREMENT（ADR-0384 决定三、决定八）。
type WarehouseMeasurementAuthenticator struct {
	minter ReceiptWorkFactMinter
}

var _ warehousehttp.WarehouseMeasurementAuthenticator = (*WarehouseMeasurementAuthenticator)(nil)

func NewWarehouseMeasurementAuthenticator(minter ReceiptWorkFactMinter) (*WarehouseMeasurementAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: warehouse measurement authenticator needs a minter")
	}
	return &WarehouseMeasurementAuthenticator{minter: minter}, nil
}

func (authenticator *WarehouseMeasurementAuthenticator) AuthenticateWarehouseMeasurement(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.MeasurementOperatorID, domain.MeasurementDeviceID, error) {
	var (
		noneTenant   domain.TenantID
		noneOperator domain.MeasurementOperatorID
		noneDevice   domain.MeasurementDeviceID
	)
	admission := identity.WorkFactAdmission(identity.WorkFactWarehouseMeasurement)
	if admission == nil {
		return noneTenant, noneOperator, noneDevice, errors.New("warehouse fulfillment accessidentity: warehouse measurement is not admitted at mint")
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
	operator, err := domain.NewMeasurementOperatorID(subject.Issuer() + "#" + subject.Subject())
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("warehouse fulfillment accessidentity: operator: %w", err)
	}
	device, err := domain.NewMeasurementDeviceID(envelope.DeviceID())
	if err != nil {
		return noneTenant, noneOperator, noneDevice, fmt.Errorf("warehouse fulfillment accessidentity: device: %w", err)
	}
	return tenant, operator, device, nil
}
