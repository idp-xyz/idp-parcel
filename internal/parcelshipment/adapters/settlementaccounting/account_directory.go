package settlementaccounting

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// RegisteredAccountDirectory 用结算账户登记册回答作用域缝要的账户。
// 这一缝形成的是货主侧接受前控制的作用域，收付方向固定为应收：应付行在同一册上，
// 但不是这一问的答案。预付或账期在结算政策的适用范围里，不另作一维。
type RegisteredAccountDirectory struct {
	accounts saports.SettlementAccountRegister
}

func NewRegisteredAccountDirectory(accounts saports.SettlementAccountRegister) (*RegisteredAccountDirectory, error) {
	if accounts == nil {
		return nil, fmt.Errorf("settlement account directory: register is nil")
	}
	return &RegisteredAccountDirectory{accounts: accounts}, nil
}

func (directory *RegisteredAccountDirectory) FindSettlementAccount(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	terms psdomain.AdoptedSettlementTerms,
) (sadomain.SettlementAccountID, bool, error) {
	tenant, err := sadomain.NewTenantID(identity.TenantID().String())
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	legalEntity, err := sadomain.NewLegalEntityReference(terms.LegalEntity().String())
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	counterparty, err := sadomain.NewSettlementCounterpartyReference(terms.Counterparty().String())
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	currency, err := sadomain.NewCurrencyCode(terms.Currency().String())
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	policy, err := sadomain.NewSettlementPolicyReference(terms.Policy().String())
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	key, err := sadomain.NewSettlementAccountKey(
		legalEntity, counterparty, sadomain.ChargeReceivable, currency, policy,
	)
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	id, found, err := directory.accounts.Find(ctx, tenant, key)
	if err != nil {
		return sadomain.SettlementAccountID{}, false, fmt.Errorf("settlement account directory: %w", err)
	}
	return id, found, nil
}
