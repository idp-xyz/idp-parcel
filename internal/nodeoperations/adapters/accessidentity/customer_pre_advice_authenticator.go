package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// CustomerPreAdviceMinter 是本适配器对铸造预报两种信封的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerPreAdviceMinter interface {
	MintDeliveryPreAdviceSubmission(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.DeliveryPreAdviceSubmissionEnvelope, error)
	MintDeliveryPreAdviceWithdrawal(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.DeliveryPreAdviceWithdrawalEnvelope, error)
}

// CustomerPreAdviceAuthenticator 实现 nodeopshttp.CustomerPreAdviceAuthenticator：按口铸首方客户族的预报信封（ADR-0139 决定二；ADR-0212
// 决定二），把四要素译成本上下文的客户身份。准入范围在铸造侧判（ADR-0139 决定八），这里只译它的格。
type CustomerPreAdviceAuthenticator struct {
	minter CustomerPreAdviceMinter
}

var _ nodeopshttp.CustomerPreAdviceAuthenticator = (*CustomerPreAdviceAuthenticator)(nil)

func NewCustomerPreAdviceAuthenticator(minter CustomerPreAdviceMinter) (*CustomerPreAdviceAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: customer pre-advice authenticator needs a minter")
	}
	return &CustomerPreAdviceAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerPreAdviceAuthenticator) AuthenticateDeliveryPreAdviceSubmission(
	ctx context.Context,
	credential nodeopshttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (nodeopshttp.CustomerIdentity, error) {
	envelope, err := authenticator.minter.MintDeliveryPreAdviceSubmission(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, answerCustomer(err)
	}
	return customerIdentityOf(envelope.Envelope())
}

func (authenticator *CustomerPreAdviceAuthenticator) AuthenticateDeliveryPreAdviceWithdrawal(
	ctx context.Context,
	credential nodeopshttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (nodeopshttp.CustomerIdentity, error) {
	envelope, err := authenticator.minter.MintDeliveryPreAdviceWithdrawal(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, answerCustomer(err)
	}
	return customerIdentityOf(envelope.Envelope())
}

func customerCredential(credential nodeopshttp.PresentedCustomerCredential) identity.IntegrationClientCredential {
	return identity.NewIntegrationClientCredential(credential.Token, credential.Certificate)
}

// channelRequestOf 把读载荷的口包成铸造器要的读请求口：交给推导口的只有接入请求标识那一格。
func channelRequestOf(accessRequestID func() (string, error)) func() (identity.ChannelRequest, error) {
	return func() (identity.ChannelRequest, error) {
		id, err := accessRequestID()
		if err != nil {
			return identity.ChannelRequest{}, err
		}
		return identity.NewChannelRequest(map[string]string{identity.AccessRequestIdentifier: id}), nil
	}
}

func customerIdentityOf(envelope identity.SourceEnvelope) (nodeopshttp.CustomerIdentity, error) {
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	account, err := domain.NewCustomerAccountReference(envelope.CustomerAccountID())
	if err != nil {
		return nodeopshttp.CustomerIdentity{}, fmt.Errorf("node operations accessidentity: customer account: %w", err)
	}
	return nodeopshttp.CustomerIdentity{Tenant: tenant, CustomerAccount: account, Channel: envelope.Source(), RequestKey: envelope.RequestKey()}, nil
}

// answerCustomer 把共享接入身份能力的格译成 nodeopshttp 的格，首方族那几格各对其格；读载荷自己的错（已是 nodeopshttp 的格）原样过。
// 推导不出来源请求键是客户系统没给接入请求标识，恢复动作是改请求，答坏载荷。
func answerCustomer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrCustomerClientCredentialRejected, err)
	case errors.Is(err, identity.ErrCustomerClientNotGranted):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrCustomerClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrCustomerClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrIdentityDependencyUnavailable, err)
	case errors.Is(err, identity.ErrRequestKeyNotDerived):
		return fmt.Errorf("%w: %w", nodeopshttp.ErrMalformedRequest, err)
	}
	return err
}
