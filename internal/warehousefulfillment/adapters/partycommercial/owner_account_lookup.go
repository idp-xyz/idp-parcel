package partycommercial

import (
	"context"
	"errors"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	wfdomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	wfports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// customerAccounts 是 party-commercial 货主客户账户册上「按租户读最新修订」那一格。
// 接口留在消费方：仓配只问在不在，不把提供方的登记结构带进自己的用例。
type customerAccounts interface {
	LoadLatestCustomerAccount(
		ctx context.Context,
		tenant pcdomain.TenantID,
		account pcdomain.CustomerAccountID,
	) (pcdomain.CustomerAccountRegistration, bool, error)
}

// OwnerAccountLookup 把仓配的货主核对接到 party-commercial 的客户账户册。
// 停用了的账户仍然算登记过：查不到只来自册上没有这一行。读不动原样交回，不折成没登记。
type OwnerAccountLookup struct {
	accounts customerAccounts
}

func NewOwnerAccountLookup(accounts customerAccounts) (*OwnerAccountLookup, error) {
	if accounts == nil {
		return nil, errors.New("warehouse fulfillment: owner account lookup needs the customer account register")
	}
	return &OwnerAccountLookup{accounts: accounts}, nil
}

func (lookup *OwnerAccountLookup) OwnerAccountRegistered(
	ctx context.Context,
	tenant wfdomain.TenantID,
	owner wfdomain.OwnerAccountID,
) (bool, error) {
	pcTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return false, fmt.Errorf("owner account lookup: %w", err)
	}
	account, err := pcdomain.NewCustomerAccountID(owner.String())
	if err != nil {
		return false, fmt.Errorf("owner account lookup: %w", err)
	}
	_, found, err := lookup.accounts.LoadLatestCustomerAccount(ctx, pcTenant, account)
	if err != nil {
		return false, fmt.Errorf("owner account lookup: %w", err)
	}
	return found, nil
}

var _ wfports.OwnerAccountLookup = (*OwnerAccountLookup)(nil)
