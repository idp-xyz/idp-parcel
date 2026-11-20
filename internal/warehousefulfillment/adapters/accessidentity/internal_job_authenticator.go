package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// InternalJobAuthenticator 以作业事实登记能力面铸库内作业的信封，并判这一种事实的准入范围。
// 能力词是 CapabilityWorkFactRegistration 的字面量，事实类型词是构造时绑上的那一种（ADR-0384 决定八）。
type InternalJobAuthenticator struct {
	minter ReceiptWorkFactMinter
	kind   identity.WorkFactKind
}

var _ warehousehttp.InternalJobAuthenticator = (*InternalJobAuthenticator)(nil)

func NewInternalJobAuthenticator(minter ReceiptWorkFactMinter, kind identity.WorkFactKind) (*InternalJobAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: internal job authenticator needs a minter")
	}
	if identity.WorkFactAdmission(kind) == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: internal job kind is not admitted at mint")
	}
	return &InternalJobAuthenticator{minter: minter, kind: kind}, nil
}

func (authenticator *InternalJobAuthenticator) AuthenticateInternalJob(
	ctx context.Context,
	bearerToken, deviceID, scopeKind, scopeReference string,
) (domain.TenantID, domain.ReceiptOperatorID, domain.ReceiptDeviceID, error) {
	var (
		noneTenant   domain.TenantID
		noneOperator domain.ReceiptOperatorID
		noneDevice   domain.ReceiptDeviceID
	)
	admission := identity.WorkFactAdmission(authenticator.kind)
	if admission == nil {
		return noneTenant, noneOperator, noneDevice, errors.New("warehouse fulfillment accessidentity: internal job is not admitted at mint")
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

// OperatorConditionRestoreAuthenticator 认证次品转良品（决定种类 CONDITION_RESTORE，准入两格同取字面量）。
type OperatorConditionRestoreAuthenticator struct {
	minter Minter
}

var _ warehousehttp.ConditionRestoreAuthenticator = (*OperatorConditionRestoreAuthenticator)(nil)

func NewOperatorConditionRestoreAuthenticator(minter Minter) (*OperatorConditionRestoreAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: condition restore authenticator needs a minter")
	}
	return &OperatorConditionRestoreAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorConditionRestoreAuthenticator) AuthenticateConditionRestore(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionConditionRestore)
}
