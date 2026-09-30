package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestAccountingConnectorOnlyAcceptsCanonicalExchange(t *testing.T) {
	counterparty := mustAccountingCounterparty(t, "SYN-SUPPLIER")
	registration, err := domain.NewAccountingConnectorRegistration(
		domain.AccountingExchangeSupplierBill, counterparty, domain.AccountingConnectorCanonical)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Form() != domain.AccountingConnectorCanonical || registration.Kind() != domain.AccountingExchangeSupplierBill {
		t.Fatalf("登记 = %s / %s", registration.Kind(), registration.Form())
	}
	if _, err := domain.AccountingConnectorFormFromName("SAP_IDOC"); !errors.Is(err, domain.ErrInvalidAccountingConnector) {
		t.Fatalf("财务系统报文 = %v", err)
	}
	if _, err := domain.AccountingExchangeKindFromName("CUSTOMER_STATEMENT"); !errors.Is(err, domain.ErrInvalidAccountingConnector) {
		t.Fatalf("未内置的交换 = %v", err)
	}
	if _, err := domain.NewAccountingConnectorRegistration(
		domain.AccountingExchangeKindInvalid, counterparty, domain.AccountingConnectorCanonical,
	); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空种类 = %v", err)
	}
}

func mustAccountingCounterparty(t *testing.T, value string) domain.AccountingCounterpartyReference {
	t.Helper()
	counterparty, err := domain.NewAccountingCounterpartyReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return counterparty
}
