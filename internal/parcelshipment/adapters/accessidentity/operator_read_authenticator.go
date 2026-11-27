package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// OperatorOperationsReadAuthenticator 实现 shipmenthttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面
// 铸信封，不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。
type OperatorOperationsReadAuthenticator struct {
	minter Minter
}

var _ shipmenthttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter Minter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("parcel shipment accessidentity: operator operations read authenticator needs a minter")
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
		return domain.TenantID{}, fmt.Errorf("parcel shipment accessidentity: tenant: %w", err)
	}
	return tenant, nil
}

// CustomerVisibility 再铸一次查阅读信封，交回租户和客户可见范围。all 为真是显式的全部客户；
// 为假时 ids 是点名的账户，空切片是什么都看不见。
func (authenticator *OperatorOperationsReadAuthenticator) CustomerVisibility(ctx context.Context, bearerToken string) (domain.TenantID, bool, []string, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return domain.TenantID{}, false, nil, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, false, nil, fmt.Errorf("parcel shipment accessidentity: tenant: %w", err)
	}
	all, ids := envelope.CustomerVisibility()
	return tenant, all, ids, nil
}
