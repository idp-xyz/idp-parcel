package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// externalFactCapability 是集成客户端生产写在治理登记册上的能力格；事实类型取外部资金事实。
// 能力词是 EXTERNAL_FACT：关务外部结果与监管凭证用的也是这一个，治理登记册上才是同一格能力。
const externalFactCapability = "EXTERNAL_FACT"

// ClientMinter 是本适配器对铸造集成客户端信封的最小依赖。
type ClientMinter interface {
	MintIntegrationClient(ctx context.Context, credential identity.IntegrationClientCredential, request identity.IntegrationClientRequest) (identity.IntegrationClientEnvelope, error)
}

// IntegrationClientAuthenticator 实现 settlementhttp.IntegrationClientAuthenticator。采用与更正都核外部资金事实这一类授予。
type IntegrationClientAuthenticator struct {
	minter ClientMinter
}

var _ settlementhttp.IntegrationClientAuthenticator = (*IntegrationClientAuthenticator)(nil)

func NewIntegrationClientAuthenticator(minter ClientMinter) (*IntegrationClientAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("settlement accessidentity: integration client authenticator needs a minter")
	}
	return &IntegrationClientAuthenticator{minter: minter}, nil
}

func (authenticator *IntegrationClientAuthenticator) AuthenticateExternalFunds(
	ctx context.Context,
	credential settlementhttp.PresentedClientCredential,
) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintIntegrationClient(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		identity.IntegrationClientRequest{
			FactType: identity.FactExternalFunds,
			Admission: &identity.AdmissionRequirement{
				Capability: externalFactCapability,
				FactKind:   identity.FactExternalFunds.String(),
			},
		})
	if err != nil {
		return domain.TenantID{}, answerClient(err)
	}
	return domain.NewTenantID(envelope.TenantID())
}

func answerClient(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", settlementhttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", settlementhttp.ErrIntegrationClientCredentialRejected, err)
	case errors.Is(err, identity.ErrIntegrationClientNotGranted):
		return fmt.Errorf("%w: %w", settlementhttp.ErrIntegrationClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", settlementhttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrIntegrationClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", settlementhttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
