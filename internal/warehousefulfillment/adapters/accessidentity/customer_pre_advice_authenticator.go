package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// CustomerInboundPreAdviceMinter 是本适配器对铸造入库预报货主口的最小依赖。identity.CustomerClientMinter 满足它。
type CustomerInboundPreAdviceMinter interface {
	MintInboundPreAdviceSubmission(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.InboundPreAdviceSubmissionEnvelope, error)
	MintInboundPreAdviceArrivalNotice(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.InboundPreAdviceArrivalNoticeEnvelope, error)
	MintInboundPreAdviceWithdrawal(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.InboundPreAdviceWithdrawalEnvelope, error)
	MintWarehouseRead(ctx context.Context, credential identity.IntegrationClientCredential) (identity.WarehouseReadEnvelope, error)
}

// CustomerInboundPreAdviceAuthenticator 把首方铸造译成仓配的客户身份。三个写口的准入词由铸造方法自己定，这里只译它的格。
type CustomerInboundPreAdviceAuthenticator struct {
	minter CustomerInboundPreAdviceMinter
}

var _ warehousehttp.CustomerInboundPreAdviceAuthenticator = (*CustomerInboundPreAdviceAuthenticator)(nil)

func NewCustomerInboundPreAdviceAuthenticator(minter CustomerInboundPreAdviceMinter) (*CustomerInboundPreAdviceAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: inbound pre-advice customer authenticator needs a minter")
	}
	return &CustomerInboundPreAdviceAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerInboundPreAdviceAuthenticator) AuthenticateInboundPreAdviceSubmission(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintInboundPreAdviceSubmission(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInboundPreAdviceAuthenticator) AuthenticateInboundPreAdviceArrivalNotice(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintInboundPreAdviceArrivalNotice(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInboundPreAdviceAuthenticator) AuthenticateInboundPreAdviceWithdrawal(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintInboundPreAdviceWithdrawal(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerInboundPreAdviceAuthenticator) AuthenticateWarehouseRead(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
) (warehousehttp.CustomerReadIdentity, error) {
	envelope, err := authenticator.minter.MintWarehouseRead(ctx, customerCredential(credential))
	if err != nil {
		return warehousehttp.CustomerReadIdentity{}, answerCustomer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return warehousehttp.CustomerReadIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	owner, err := domain.NewOwnerAccountID(envelope.CustomerAccountID())
	if err != nil {
		return warehousehttp.CustomerReadIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: owner: %w", err)
	}
	return warehousehttp.CustomerReadIdentity{Tenant: tenant, Owner: owner}, nil
}

func customerCredential(credential warehousehttp.PresentedCustomerCredential) identity.IntegrationClientCredential {
	return identity.NewIntegrationClientCredential(credential.Token, credential.Certificate)
}

func channelRequestOf(accessRequestID func() (string, error)) func() (identity.ChannelRequest, error) {
	return func() (identity.ChannelRequest, error) {
		id, err := accessRequestID()
		if err != nil {
			return identity.ChannelRequest{}, err
		}
		return identity.NewChannelRequest(map[string]string{identity.AccessRequestIdentifier: id}), nil
	}
}

func commandIdentity(envelope identity.SourceEnvelope) (warehousehttp.CustomerCommandIdentity, error) {
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	owner, err := domain.NewOwnerAccountID(envelope.CustomerAccountID())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: owner: %w", err)
	}
	source, err := domain.NewSubmissionSource(envelope.Source())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: source: %w", err)
	}
	key, err := domain.NewRequestKey(envelope.RequestKey())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, fmt.Errorf("%w: %w", warehousehttp.ErrMalformedRequest, err)
	}
	// 货主口没有另一个操作者。登记责任方取信封上的客户渠道来源，与提交来源同一格；读口不带出这一格。
	registrant, err := domain.NewRegistrant(envelope.Source())
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, fmt.Errorf("warehouse fulfillment accessidentity: registrant: %w", err)
	}
	return warehousehttp.CustomerCommandIdentity{
		Tenant: tenant, Owner: owner, Source: source, RequestKey: key, Registrant: registrant,
	}, nil
}

func answerCustomer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", warehousehttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", warehousehttp.ErrCustomerClientCredentialRejected, err)
	case errors.Is(err, identity.ErrCustomerClientNotGranted):
		return fmt.Errorf("%w: %w", warehousehttp.ErrCustomerClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", warehousehttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrCustomerClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", warehousehttp.ErrIdentityDependencyUnavailable, err)
	case errors.Is(err, identity.ErrRequestKeyNotDerived):
		return fmt.Errorf("%w: %w", warehousehttp.ErrMalformedRequest, err)
	case errors.Is(err, warehousehttp.ErrMalformedRequest):
		return err
	}
	return err
}
