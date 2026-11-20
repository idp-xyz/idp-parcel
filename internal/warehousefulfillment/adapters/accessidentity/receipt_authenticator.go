package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ReceiptWorkFactMinter 是收货确认对铸造作业事实信封的最小依赖。identity.WorkFactMinter 满足它。
type ReceiptWorkFactMinter interface {
	MintWorkFact(ctx context.Context, credential identity.OperatorCredential, request identity.WorkFactRequest) (identity.WorkFactEnvelope, error)
}

// ReceiptConfirmationAuthenticator 以作业事实登记能力面铸收货确认的信封，并判这一种事实的准入范围。
// 能力词是 CapabilityWorkFactRegistration 的字面量，事实类型词是 RECEIPT_CONFIRMATION（ADR-0384 决定八）。
// 作业范围由调用方给出：种类必须已经是仓，引用等于载荷里的仓。
type ReceiptConfirmationAuthenticator struct {
	minter ReceiptWorkFactMinter
}

var _ warehousehttp.ReceiptConfirmationAuthenticator = (*ReceiptConfirmationAuthenticator)(nil)

func NewReceiptConfirmationAuthenticator(minter ReceiptWorkFactMinter) (*ReceiptConfirmationAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: receipt confirmation authenticator needs a minter")
	}
	return &ReceiptConfirmationAuthenticator{minter: minter}, nil
}

func (authenticator *ReceiptConfirmationAuthenticator) AuthenticateReceiptConfirmation(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	var (
		noneTenant   domain.TenantID
		noneOperator domain.ReceiptOperatorID
		noneDevice   domain.ReceiptDeviceID
	)
	admission := identity.WorkFactAdmission(identity.WorkFactReceiptConfirmation)
	if admission == nil {
		return noneTenant, noneOperator, noneDevice, errors.New("warehouse fulfillment accessidentity: receipt confirmation is not admitted at mint")
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
	// 操作者标识与运营决定面同一写法：发行方与 sub 用 # 接上。批准职责规则才能把两个面的操作者比成同一个人（ADR-0384 决定四）。
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

// answerReceipt 在登记写面的译法之上多译设备与准入范围：收货确认会遇到这两格，登记册配置写不会。
func answerReceipt(err error) error {
	switch {
	case errors.Is(err, identity.ErrDeviceNotInUse):
		return fmt.Errorf("%w: %w", warehousehttp.ErrDeviceNotInUse, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", warehousehttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrAdmissionScopeUnavailable),
		errors.Is(err, identity.ErrDeviceRegistryUnavailable):
		return fmt.Errorf("%w: %w", warehousehttp.ErrIdentityDependencyUnavailable, err)
	default:
		return answer(err)
	}
}
