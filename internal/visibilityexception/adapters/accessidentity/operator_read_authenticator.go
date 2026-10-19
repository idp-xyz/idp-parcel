package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// OperatorOperationsReadAuthenticator 实现 visibilityhttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面
// 铸信封，不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。答复格的翻译与登记写面共用 answer。
type OperatorOperationsReadAuthenticator struct {
	minter Minter
}

var _ visibilityhttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter Minter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("visibility accessidentity: operator operations read authenticator needs a minter")
	}
	return &OperatorOperationsReadAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorOperationsReadAuthenticator) AuthenticateOperationsRead(ctx context.Context, bearerToken string) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return domain.TenantID{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("visibility accessidentity: tenant: %w", err)
	}
	return tenant, nil
}
