package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// OperatorRegistryAuthenticator 实现 nodeopshttp.OperatorRegistryAuthenticator：以登记册配置写能力面铸信封，不带准入要求——登记一行
// 不形成任何生产事实（ADR-0100 决定四）。答复格的翻译与查阅读共用 answerOperationsRead：两面都不核设备、不判准入范围，会出现的格
// 一样多。
type OperatorRegistryAuthenticator struct {
	minter Minter
}

var _ nodeopshttp.OperatorRegistryAuthenticator = (*OperatorRegistryAuthenticator)(nil)

func NewOperatorRegistryAuthenticator(minter Minter) (*OperatorRegistryAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("node operations accessidentity: operator registry authenticator needs a minter")
	}
	return &OperatorRegistryAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorRegistryAuthenticator) AuthenticateRegistryWrite(ctx context.Context, bearerToken string) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityRegistryConfigurationWrite})
	if err != nil {
		return domain.TenantID{}, answerOperationsRead(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("node operations accessidentity: tenant: %w", err)
	}
	return tenant, nil
}
