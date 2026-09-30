package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// AccountingConnectorRegister 写下某个对方的一种交换采用规范文书。读口是 AccountingConnectorView。
// 不预列供应商或财务系统。
type AccountingConnectorRegister interface {
	SaveAccountingConnector(
		ctx context.Context,
		tenant domain.TenantID,
		registration domain.AccountingConnectorRegistration,
		at time.Time,
	) (CatalogueRegistrationEffect, error)
}

// AccountingConnectorView 按交换种类与对方取已采用的连接器形态。found=false 表示没有这一行：
// 外部交换答未配置，不解析报文。
type AccountingConnectorView interface {
	LoadAccountingConnector(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.AccountingExchangeKind,
		counterparty domain.AccountingCounterpartyReference,
	) (domain.AccountingConnectorForm, bool, error)
}
