package accessidentity

import (
	"context"
	"errors"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// OperatorRegistryAuthenticator 实现 tfhttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封，不带准入要求
// （ADR-0100 决定四）。答复格的翻译与运营决定口共用 answer。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ tfhttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("transport fulfillment accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return domain.TenantID{}, answer(err)
	}
	return domain.NewTenantID(envelope.TenantID())
}
