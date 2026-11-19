package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OperatorOperationsReadAuthenticator 实现 warehousehttp.OperatorOperationsReadAuthenticator：以主数据与运营查阅读能力面铸
// 信封，交回租户。不带准入要求——查阅不形成生产事实（ADR-0100 决定四）。持登记册配置写不等于持查阅读：两格是册上平列的授予，
// 只持写的操作者在这一面答未授予。
type OperatorOperationsReadAuthenticator struct {
	minter Minter
}

var _ warehousehttp.OperatorOperationsReadAuthenticator = (*OperatorOperationsReadAuthenticator)(nil)

func NewOperatorOperationsReadAuthenticator(minter Minter) (*OperatorOperationsReadAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: operator operations read authenticator needs a minter")
	}
	return &OperatorOperationsReadAuthenticator{minter: minter}, nil
}

func (authenticator *OperatorOperationsReadAuthenticator) AuthenticateOperationsRead(
	ctx context.Context,
	bearerToken string,
) (domain.TenantID, error) {
	envelope, err := authenticator.minter.MintOperator(ctx, identity.NewOperatorCredential(bearerToken),
		identity.OperatorRequest{Face: identity.CapabilityMasterDataAndOperationsRead})
	if err != nil {
		return domain.TenantID{}, answer(err)
	}
	tenant, err := domain.NewTenantID(envelope.TenantID())
	if err != nil {
		return domain.TenantID{}, fmt.Errorf("warehouse fulfillment accessidentity: tenant: %w", err)
	}
	return tenant, nil
}
