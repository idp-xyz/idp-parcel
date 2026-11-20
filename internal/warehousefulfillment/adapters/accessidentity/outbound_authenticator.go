package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// CustomerOutboundMinter 是出库请求货主口要的铸造。
type CustomerOutboundMinter interface {
	MintOutboundRequestSubmission(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.OutboundRequestSubmissionEnvelope, error)
	MintOutboundRequestSplit(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.OutboundRequestSubmissionEnvelope, error)
}

// CustomerOutboundAuthenticator 把首方铸造译成仓配的客户身份。
type CustomerOutboundAuthenticator struct {
	minter CustomerOutboundMinter
}

var _ warehousehttp.CustomerOutboundAuthenticator = (*CustomerOutboundAuthenticator)(nil)

func NewCustomerOutboundAuthenticator(minter CustomerOutboundMinter) (*CustomerOutboundAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: outbound customer authenticator needs a minter")
	}
	return &CustomerOutboundAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerOutboundAuthenticator) AuthenticateOutboundRequestSubmission(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.authenticate(ctx, credential, accessRequestID, authenticator.minter.MintOutboundRequestSubmission)
}

func (authenticator *CustomerOutboundAuthenticator) AuthenticateOutboundRequestSplit(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return authenticator.authenticate(ctx, credential, accessRequestID, authenticator.minter.MintOutboundRequestSplit)
}

func (authenticator *CustomerOutboundAuthenticator) authenticate(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
	mint func(context.Context, identity.IntegrationClientCredential, func() (identity.ChannelRequest, error)) (identity.OutboundRequestSubmissionEnvelope, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := mint(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

// OperatorOutboundAuthenticator 认证出库请求代录。
type OperatorOutboundAuthenticator struct {
	minter Minter
}

var _ warehousehttp.OutboundOnBehalfAuthenticator = (*OperatorOutboundAuthenticator)(nil)

func NewOperatorOutboundAuthenticator(minter Minter) (*OperatorOutboundAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: outbound authenticator needs a minter")
	}
	return &OperatorOutboundAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorOutboundAuthenticator) AuthenticateOutboundOnBehalf(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, identity.DecisionOutboundRequestRecordOnBehalf)
}
