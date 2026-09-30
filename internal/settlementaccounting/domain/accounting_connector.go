package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidAccountingConnector 说明交换种类或连接器形态不是这套内置形态认得的值。
	ErrInvalidAccountingConnector = errors.New("settlement accounting: invalid accounting connector")
)

// AccountingExchangeKind 是规范文书交换的种类。供应商账单与外部资金事实各是一种，不互相顶替。
type AccountingExchangeKind uint8

const (
	AccountingExchangeKindInvalid AccountingExchangeKind = iota
	AccountingExchangeSupplierBill
	AccountingExchangeExternalFunds
)

func (kind AccountingExchangeKind) String() string {
	switch kind {
	case AccountingExchangeSupplierBill:
		return "SUPPLIER_BILL"
	case AccountingExchangeExternalFunds:
		return "EXTERNAL_FUNDS"
	default:
		return ""
	}
}

func (kind AccountingExchangeKind) admits() bool {
	return kind == AccountingExchangeSupplierBill || kind == AccountingExchangeExternalFunds
}

// AccountingExchangeKindFromName 只认封闭的两种交换。词表外拒，不夹成其中一种。
func AccountingExchangeKindFromName(name string) (AccountingExchangeKind, error) {
	switch name {
	case "SUPPLIER_BILL":
		return AccountingExchangeSupplierBill, nil
	case "EXTERNAL_FUNDS":
		return AccountingExchangeExternalFunds, nil
	default:
		return AccountingExchangeKindInvalid, fmt.Errorf("%w: exchange kind", ErrInvalidAccountingConnector)
	}
}

// AccountingConnectorForm 是内置连接器形态。只有规范文书：载荷已经是产品自己的账单主张或外部资金事实。
// 不带任何供应商账单版式，也不带任何财务系统的报文。
type AccountingConnectorForm uint8

const (
	AccountingConnectorFormInvalid AccountingConnectorForm = iota
	AccountingConnectorCanonical
)

func (form AccountingConnectorForm) String() string {
	switch form {
	case AccountingConnectorCanonical:
		return "CANONICAL"
	default:
		return ""
	}
}

func (form AccountingConnectorForm) admits() bool {
	return form == AccountingConnectorCanonical
}

// AccountingConnectorFormFromName 只认规范文书。词表外拒，不夹成规范文书。
func AccountingConnectorFormFromName(name string) (AccountingConnectorForm, error) {
	switch name {
	case "CANONICAL":
		return AccountingConnectorCanonical, nil
	default:
		return AccountingConnectorFormInvalid, fmt.Errorf("%w: form", ErrInvalidAccountingConnector)
	}
}

// AccountingCounterpartyReference 指名这次交换的对方。供应商或财务系统的身份是租户的词，不在产品里预列。
type AccountingCounterpartyReference struct{ requiredValue }

func NewAccountingCounterpartyReference(value string) (AccountingCounterpartyReference, error) {
	required, err := newRequiredValue("accounting counterparty", value)
	return AccountingCounterpartyReference{required}, err
}

// AccountingConnectorRegistration 把一个对方的一种交换挂到规范文书上。
type AccountingConnectorRegistration struct {
	kind         AccountingExchangeKind
	counterparty AccountingCounterpartyReference
	form         AccountingConnectorForm
}

func NewAccountingConnectorRegistration(
	kind AccountingExchangeKind,
	counterparty AccountingCounterpartyReference,
	form AccountingConnectorForm,
) (AccountingConnectorRegistration, error) {
	if !kind.admits() || !counterparty.valid() || !form.admits() {
		return AccountingConnectorRegistration{}, fmt.Errorf("%w: accounting connector", ErrBlankValue)
	}
	return AccountingConnectorRegistration{kind: kind, counterparty: counterparty, form: form}, nil
}

func (registration AccountingConnectorRegistration) Kind() AccountingExchangeKind {
	return registration.kind
}

func (registration AccountingConnectorRegistration) Counterparty() AccountingCounterpartyReference {
	return registration.counterparty
}

func (registration AccountingConnectorRegistration) Form() AccountingConnectorForm {
	return registration.form
}
