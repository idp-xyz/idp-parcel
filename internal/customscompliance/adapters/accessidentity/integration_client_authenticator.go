package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// ClientMinter 是本适配器对铸造集成客户端信封的最小依赖。
type ClientMinter interface {
	MintIntegrationClient(ctx context.Context, credential identity.IntegrationClientCredential, request identity.IntegrationClientRequest) (identity.IntegrationClientEnvelope, error)
}

// IntegrationClientAuthenticator 实现 customshttp.IntegrationClientAuthenticator：按本口的事实类型铸信封，并判准入范围。
type IntegrationClientAuthenticator struct {
	minter ClientMinter
}

var (
	_ customshttp.IntegrationClientAuthenticator = (*IntegrationClientAuthenticator)(nil)
	_ customshttp.ManifestSourceAuthenticator    = (*IntegrationClientAuthenticator)(nil)
)

func NewIntegrationClientAuthenticator(minter ClientMinter) (*IntegrationClientAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("customs accessidentity: integration client authenticator needs a minter")
	}
	return &IntegrationClientAuthenticator{minter: minter}, nil
}

func (authenticator *IntegrationClientAuthenticator) Authenticate(
	ctx context.Context,
	credential customshttp.PresentedClientCredential,
	fact customshttp.IntegrationClientFact,
) (domain.TenantID, error) {
	kind, err := identity.ParseExternalFactType(string(fact))
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("customs accessidentity: fact %q: %w", fact, err)
	}
	envelope, err := authenticator.minter.MintIntegrationClient(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		identity.IntegrationClientRequest{
			FactType:  kind,
			Admission: &identity.AdmissionRequirement{Capability: identity.ExternalFactAdmissionCapability, FactKind: kind.String()},
		})
	if err != nil {
		return domain.TenantID{}, answerClient(err)
	}
	return domain.NewTenantID(envelope.TenantID())
}

// AuthenticateManifestSource 交回信封上的租户与来源身份。来源身份是册上绑定的那一个：交舱单引用的客户端代表哪家承运商
// 由它定，关务的 Intake 把它记成引用上的承运商责任。
func (authenticator *IntegrationClientAuthenticator) AuthenticateManifestSource(
	ctx context.Context,
	credential customshttp.PresentedClientCredential,
) (customshttp.ManifestSource, error) {
	envelope, err := authenticator.minter.MintIntegrationClient(ctx,
		identity.NewIntegrationClientCredential(credential.Token, credential.Certificate),
		identity.IntegrationClientRequest{
			FactType: identity.FactExternalRegulatoryManifest,
			Admission: &identity.AdmissionRequirement{
				Capability: identity.ExternalFactAdmissionCapability,
				FactKind:   identity.FactExternalRegulatoryManifest.String(),
			},
		})
	if err != nil {
		return customshttp.ManifestSource{}, answerClient(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return customshttp.ManifestSource{}, fmt.Errorf("customs accessidentity: tenant: %w", err)
	}
	return customshttp.ManifestSource{Tenant: tenant, Carrier: envelope.SourceIdentity()}, nil
}

func answerClient(err error) error {
	switch {
	case errors.Is(err, identity.ErrAccessChannelNotConfigured):
		return fmt.Errorf("%w: %w", customshttp.ErrAccessChannelNotConfigured, err)
	case errors.Is(err, identity.ErrCredentialRejected):
		return fmt.Errorf("%w: %w", customshttp.ErrIntegrationClientCredentialRejected, err)
	case errors.Is(err, identity.ErrIntegrationClientNotGranted):
		return fmt.Errorf("%w: %w", customshttp.ErrIntegrationClientNotGranted, err)
	case errors.Is(err, identity.ErrOutsideAdmissionScope):
		return fmt.Errorf("%w: %w", customshttp.ErrOutsideAdmissionScope, err)
	case errors.Is(err, identity.ErrCredentialVerifierUnavailable),
		errors.Is(err, identity.ErrIntegrationClientRegistryUnavailable),
		errors.Is(err, identity.ErrAdmissionScopeUnavailable):
		return fmt.Errorf("%w: %w", customshttp.ErrIdentityDependencyUnavailable, err)
	}
	return err
}
