package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type accountingConnectorDocument struct {
	TenantID     string `json:"tenantId"`
	ExchangeKind string `json:"exchangeKind"`
	Counterparty string `json:"counterparty"`
	Form         string `json:"form"`
}

// AccountingConnectorFromJSON 译装一条连接器选用。不预填对方，也不解析报文。
func AccountingConnectorFromJSON(raw []byte) (domain.TenantID, domain.AccountingConnectorRegistration, error) {
	var document accountingConnectorDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, fmt.Errorf("账务连接器登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	kind, err := domain.AccountingExchangeKindFromName(document.ExchangeKind)
	if err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, fmt.Errorf("exchangeKind：%w", err)
	}
	counterparty, err := domain.NewAccountingCounterpartyReference(document.Counterparty)
	if err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, fmt.Errorf("counterparty：%w", err)
	}
	form, err := domain.AccountingConnectorFormFromName(document.Form)
	if err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, fmt.Errorf("form：%w", err)
	}
	registration, err := domain.NewAccountingConnectorRegistration(kind, counterparty, form)
	if err != nil {
		return domain.TenantID{}, domain.AccountingConnectorRegistration{}, err
	}
	return tenant, registration, nil
}
