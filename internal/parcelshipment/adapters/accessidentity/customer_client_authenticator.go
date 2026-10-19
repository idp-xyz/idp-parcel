package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// CustomerSubmissionMinter 是本适配器对铸造首方提交信封的最小依赖，identity.CustomerClientMinter 满足它。
type CustomerSubmissionMinter interface {
	MintSubmission(
		ctx context.Context,
		credential identity.IntegrationClientCredential,
		readRequest func() (identity.ChannelRequest, error),
	) (identity.SubmissionEnvelope, error)
}

// CustomerClientAuthenticator 实现 shipmenthttp.CustomerClientAuthenticator：铸首方客户族的提交信封（ADR-0139 决定二），
// 把四要素译成本上下文的来源身份。提交口不在铸造侧判准入范围（ADR-0139 决定八），生产归属由编排判。
type CustomerClientAuthenticator struct {
	minter CustomerSubmissionMinter
}

var _ shipmenthttp.CustomerClientAuthenticator = (*CustomerClientAuthenticator)(nil)

func NewCustomerClientAuthenticator(minter CustomerSubmissionMinter) (*CustomerClientAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("parcel shipment accessidentity: customer client authenticator needs a minter")
	}
	return &CustomerClientAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerClientAuthenticator) AuthenticateSubmission(
	ctx context.Context,
	credential shipmenthttp.PresentedClientCredential,
	accessRequestID func() (string, error),
) (domain.SourceIdentity, error) {
	envelope, err := authenticator.minter.MintSubmission(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		func() (identity.ChannelRequest, error) {
			id, err := accessRequestID()
			if err != nil {
				return identity.ChannelRequest{}, err
			}
			return identity.NewChannelRequest(map[string]string{identity.AccessRequestIdentifier: id}), nil
		})
	if err != nil {
		return domain.SourceIdentity{}, answerCustomer(err)
	}
	minted := envelope.Envelope()
	tenant, err := domain.NewTenantID(minted.TenantID())
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("parcel shipment accessidentity: tenant: %w", err)
	}
	account, err := domain.NewCustomerAccountID(minted.CustomerAccountID())
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("parcel shipment accessidentity: customer account: %w", err)
	}
	source, err := domain.NewSource(minted.Source())
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("parcel shipment accessidentity: source: %w", err)
	}
	key, err := domain.NewSourceRequestKey(minted.RequestKey())
	if err != nil {
		return domain.SourceIdentity{}, fmt.Errorf("parcel shipment accessidentity: source request key: %w", err)
	}
	return domain.NewSourceIdentity(tenant, account, source, key)
}

// answerCustomer 把共享接入身份能力的格译成 shipmenthttp 的格，首方族那几格各对其格；读载荷自己的错（已是本包的格）原样
// 过。推导不出来源请求键是客户系统没给接入请求标识，恢复动作是改请求，答形状错。
func answerCustomer(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrCustomerClientCredentialRejected, err)
	case errors.Is(err, identity.ErrCustomerClientNotGranted):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrCustomerClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrCustomerClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrIdentityDependencyUnavailable, err)
	case errors.Is(err, identity.ErrRequestKeyNotDerived):
		return fmt.Errorf("%w: %w", shipmenthttp.ErrMalformedRequest, err)
	}
	return err
}
