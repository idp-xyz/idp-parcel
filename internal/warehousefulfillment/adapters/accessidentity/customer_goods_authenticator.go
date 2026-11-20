package accessidentity

import (
	"context"
	"errors"
	"fmt"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// CustomerGoodsMinter 是本适配器对铸造货主商品维护与查阅口的最小依赖。identity.CustomerClientMinter 满足它。
type CustomerGoodsMinter interface {
	MintGoodsMaintenance(ctx context.Context, credential identity.IntegrationClientCredential, readRequest func() (identity.ChannelRequest, error)) (identity.GoodsMaintenanceEnvelope, error)
	MintWarehouseRead(ctx context.Context, credential identity.IntegrationClientCredential) (identity.WarehouseReadEnvelope, error)
}

// CustomerGoodsAuthenticator 把首方铸造译成仓配的商品客户身份。写口准入两格都是 GOODS_MAINTENANCE，查阅不判准入。
type CustomerGoodsAuthenticator struct {
	minter CustomerGoodsMinter
}

var _ warehousehttp.CustomerGoodsAuthenticator = (*CustomerGoodsAuthenticator)(nil)

func NewCustomerGoodsAuthenticator(minter CustomerGoodsMinter) (*CustomerGoodsAuthenticator, error) {
	if minter == nil {
		return nil, errors.New("warehouse fulfillment accessidentity: goods customer authenticator needs a minter")
	}
	return &CustomerGoodsAuthenticator{minter: minter}, nil
}

func (authenticator *CustomerGoodsAuthenticator) AuthenticateGoodsMaintenance(
	ctx context.Context,
	credential warehousehttp.PresentedCustomerCredential,
	accessRequestID func() (string, error),
) (warehousehttp.CustomerCommandIdentity, error) {
	envelope, err := authenticator.minter.MintGoodsMaintenance(ctx, customerCredential(credential), channelRequestOf(accessRequestID))
	if err != nil {
		return warehousehttp.CustomerCommandIdentity{}, answerCustomer(err)
	}
	return commandIdentity(envelope.Envelope())
}

func (authenticator *CustomerGoodsAuthenticator) AuthenticateWarehouseRead(
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
