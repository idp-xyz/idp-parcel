package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// ClientMinter 是本适配器对铸造集成客户端信封的最小依赖，identity.IntegrationClientMinter 满足它。
type ClientMinter interface {
	MintIntegrationClient(ctx context.Context, credential identity.IntegrationClientCredential, request identity.IntegrationClientRequest) (identity.IntegrationClientEnvelope, error)
}

// IntegrationClientAuthenticator 实现 tfhttp.IntegrationClientAuthenticator：按承运订舱答复这一类事实铸信封，并判准入范围
// （答复形成生产事实，照 ADR-0149 决定四判）。
type IntegrationClientAuthenticator struct {
	minter ClientMinter
}

var _ tfhttp.IntegrationClientAuthenticator = (*IntegrationClientAuthenticator)(nil)

func NewIntegrationClientAuthenticator(minter ClientMinter) (*IntegrationClientAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("transport fulfillment accessidentity: integration client authenticator needs a minter")
	}
	return &IntegrationClientAuthenticator{minter: minter}, nil
}

// AuthenticateCarrierBookingAnswer 交回信封上的租户与来源身份。来源身份是册上绑定的那一个，编排拿它与订舱所属委托的
// 服务提供方比（ADR-0202 决定二）；这里不比——哪次订舱归谁，接入身份不知道。
func (authenticator *IntegrationClientAuthenticator) AuthenticateCarrierBookingAnswer(
	ctx context.Context,
	credential tfhttp.PresentedClientCredential,
) (tfhttp.CarrierAnswerIdentity, error) {
	envelope, err := authenticator.minter.MintIntegrationClient(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		identity.IntegrationClientRequest{
			FactType: identity.FactCarrierBookingAnswer,
			Admission: &identity.AdmissionRequirement{
				Capability: identity.ExternalFactAdmissionCapability,
				FactKind:   identity.FactCarrierBookingAnswer.String(),
			},
		})
	if err != nil {
		return tfhttp.CarrierAnswerIdentity{}, answerClient(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return tfhttp.CarrierAnswerIdentity{}, fmt.Errorf("transport fulfillment accessidentity: tenant: %w", err)
	}
	return tfhttp.CarrierAnswerIdentity{Tenant: tenant, Source: envelope.SourceIdentity()}, nil
}

// answerClient 把共享接入身份能力的格译成 tfhttp 的集成客户端格：处理器只认本上下文的哨兵。
func answerClient(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", tfhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", tfhttp.ErrIntegrationClientCredentialRejected, err)
	case errors.Is(err, identity.ErrIntegrationClientNotGranted):
		return fmt.Errorf("%w: %w", tfhttp.ErrIntegrationClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", tfhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrIntegrationClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", tfhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
