package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// CustomerInstructionMinter 是加工与换标四个货主口要的铸造。准入词由铸造方法自己定。
type CustomerInstructionMinter interface {
	MintProcessingInstructionSubmission(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.ProcessingInstructionSubmissionEnvelope, error)
	MintProcessingInstructionWithdrawal(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.ProcessingInstructionWithdrawalEnvelope, error)
	MintRelabelInstructionSubmission(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.RelabelInstructionSubmissionEnvelope, error)
	MintRelabelInstructionWithdrawal(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.RelabelInstructionWithdrawalEnvelope, error)
}

// CustomerInstructionAuthenticator 把首方铸造译成仓配的客户身份。
type CustomerInstructionAuthenticator struct {
	minter CustomerInstructionMinter
}

var _ warehousehttp.CustomerInstructionAuthenticator = (*CustomerInstructionAuthenticator)(nil)

func NewCustomerInstructionAuthenticator(minter CustomerInstructionMinter) (*CustomerInstructionAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: instruction customer authenticator needs a minter")
	}
	return &CustomerInstructionAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerInstructionAuthenticator) AuthenticateProcessingInstructionSubmission(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintProcessingInstructionSubmission(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInstructionAuthenticator) AuthenticateProcessingInstructionWithdrawal(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintProcessingInstructionWithdrawal(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInstructionAuthenticator) AuthenticateRelabelInstructionSubmission(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintRelabelInstructionSubmission(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInstructionAuthenticator) AuthenticateRelabelInstructionWithdrawal(ctx context.Context, credential warehousehttp.PresentedCustomerCredential, accessRequestID func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintRelabelInstructionWithdrawal(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

// OperatorInstructionAuthenticator 认证加工或换标指令。发起与作废共用一种决定授予。
type OperatorInstructionAuthenticator struct {
	minter Minter
	kind   identity.DecisionKind
}

func NewOperatorInstructionAuthenticator(minter Minter, kind identity.DecisionKind) (*OperatorInstructionAuthenticator, error) {
	if minter == nil || kind.String() == "" {
		return nil, errors.New("warehouse fulfillment accessidentity: instruction authenticator needs a minter and a decision kind")
	}
	return &OperatorInstructionAuthenticator{minter: minter, kind: kind}, nil
}

func (authenticator *OperatorInstructionAuthenticator) AuthenticateInstruction(ctx context.Context, bearerToken string) (warehousehttp.OperatorIdentity, error) {
	return mintWarehouseDecision(ctx, authenticator.minter, bearerToken, authenticator.kind)
}
