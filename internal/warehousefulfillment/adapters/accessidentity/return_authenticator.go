package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// CustomerReturnMinter 是退货货主口要的铸造。
type CustomerReturnMinter interface {
	MintReturnPreAdvice(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.ReturnPreAdviceEnvelope, error)
	MintReturnDisposition(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.ReturnDispositionEnvelope, error)
}

// CustomerReturnAuthenticator 把首方铸造译成仓配的客户身份。
type CustomerReturnAuthenticator struct {
	minter CustomerReturnMinter
}

var _ warehousehttp.CustomerReturnAuthenticator = (*CustomerReturnAuthenticator)(nil)

func NewCustomerReturnAuthenticator(minter CustomerReturnMinter) (*CustomerReturnAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: return customer authenticator needs a minter")
	}
	return &CustomerReturnAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerReturnAuthenticator) AuthenticateReturnPreAdvice(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintReturnPreAdvice(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerReturnAuthenticator) AuthenticateReturnDisposition(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintReturnDisposition(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

// OperatorReturnAuthenticator 认证退货代录。
type OperatorReturnAuthenticator struct {
	minter Minter
}

var _ warehousehttp.ReturnOnBehalfAuthenticator = (*OperatorReturnAuthenticator)(nil)

func NewOperatorReturnAuthenticator(minter Minter) (*OperatorReturnAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: return authenticator needs a minter")
	}
	return &OperatorReturnAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorReturnAuthenticator) AuthenticateReturnOnBehalf(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionReturnRecordOnBehalf)
}
