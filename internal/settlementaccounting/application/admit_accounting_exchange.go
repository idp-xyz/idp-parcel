package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AccountingConnectorAdmission 是一次外部交换能不能进规范文书。没登记不接收、不导出。
type AccountingConnectorAdmission uint8

const (
	AccountingConnectorAdmissionInvalid AccountingConnectorAdmission = iota
	AccountingConnectorUnconfigured
	AccountingConnectorAdmitted
)

func (admission AccountingConnectorAdmission) String() string {
	switch admission {
	case AccountingConnectorUnconfigured:
		return "ACCOUNTING_CONNECTOR_UNCONFIGURED"
	case AccountingConnectorAdmitted:
		return "ACCOUNTING_CONNECTOR_ADMITTED"
	default:
		return ""
	}
}

// AdmitAccountingExchangeHandler 按登记册决定这次外部交换走不走规范文书。
// 它不解析供应商账单文件，也不解析财务系统报文，也不调用账单接收或资金事实采用。
type AdmitAccountingExchangeHandler struct {
	connectors ports.AccountingConnectorView
}

func NewAdmitAccountingExchangeHandler(connectors ports.AccountingConnectorView) (*AdmitAccountingExchangeHandler, error) {
	if connectors == nil {
		return nil, fmt.Errorf("%w: accounting connector view", ErrNilDependency)
	}
	return &AdmitAccountingExchangeHandler{connectors: connectors}, nil
}

func (handler *AdmitAccountingExchangeHandler) Admit(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.AccountingExchangeKind,
	counterparty domain.AccountingCounterpartyReference,
) (AccountingConnectorAdmission, error) {
	form, found, err := handler.connectors.LoadAccountingConnector(ctx, tenant, kind, counterparty)
	if err != nil {
		return AccountingConnectorAdmissionInvalid, fmt.Errorf("admit accounting exchange: %w", err)
	}
	if !found || form != domain.AccountingConnectorCanonical {
		return AccountingConnectorUnconfigured, nil
	}
	return AccountingConnectorAdmitted, nil
}

// RegisterAccountingConnectorHandler 登记某个对方的一种交换采用规范文书。落库时刻不是载荷的一格。
type RegisterAccountingConnectorHandler struct {
	connectors ports.AccountingConnectorRegister
	clock      ports.Clock
}

func NewRegisterAccountingConnectorHandler(
	connectors ports.AccountingConnectorRegister,
	clock ports.Clock,
) (*RegisterAccountingConnectorHandler, error) {
	if connectors == nil || clock == nil {
		return nil, fmt.Errorf("%w: accounting connector register", ErrNilDependency)
	}
	return &RegisterAccountingConnectorHandler{connectors: connectors, clock: clock}, nil
}

func (handler *RegisterAccountingConnectorHandler) RegisterAccountingConnector(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AccountingConnectorRegistration,
) (ports.CatalogueRegistrationEffect, error) {
	if tenant.String() == "" {
		return 0, fmt.Errorf("%w: tenant", domain.ErrBlankValue)
	}
	return handler.connectors.SaveAccountingConnector(ctx, tenant, registration, handler.clock.Now())
}
