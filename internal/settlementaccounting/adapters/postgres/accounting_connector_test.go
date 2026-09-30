package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestAccountingConnectorsRefuseToRunOutsideATransaction(t *testing.T) {
	connectors := newAccountingConnectors(t)
	registration := accountingConnectorRegistration(t, domain.AccountingExchangeSupplierBill, "SYN-SUPPLIER")

	if _, err := connectors.SaveAccountingConnector(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestAnUnregisteredAccountingConnectorStaysUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	connectors := mustAccountingConnectors(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	supplier := accountingConnectorRegistration(t, domain.AccountingExchangeSupplierBill, "SYN-SUPPLIER")
	admit, err := application.NewAdmitAccountingExchangeHandler(connectors)
	if err != nil {
		t.Fatal(err)
	}

	admission, err := admit.Admit(t.Context(), tenant, supplier.Kind(), supplier.Counterparty())
	if err != nil || admission != application.AccountingConnectorUnconfigured {
		t.Fatalf("空册 = %s err=%v，want 未配置", admission, err)
	}

	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := connectors.SaveAccountingConnector(ctx, tenant, supplier, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := connectors.SaveAccountingConnector(ctx, tenant, supplier, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	admission, err = admit.Admit(t.Context(), tenant, supplier.Kind(), supplier.Counterparty())
	if err != nil || admission != application.AccountingConnectorAdmitted {
		t.Fatalf("已登记 = %s err=%v，want 放行规范文书", admission, err)
	}
	other := accountingConnectorRegistration(t, domain.AccountingExchangeExternalFunds, "SYN-LEDGER")
	admission, err = admit.Admit(t.Context(), tenant, other.Kind(), other.Counterparty())
	if err != nil || admission != application.AccountingConnectorUnconfigured {
		t.Fatalf("另一对方 = %s err=%v，want 未配置", admission, err)
	}
}

func newAccountingConnectors(t *testing.T) *adapter.AccountingConnectors {
	t.Helper()
	return mustAccountingConnectors(t, catalogueDB(t))
}

func mustAccountingConnectors(t *testing.T, db *bentopg.DB) *adapter.AccountingConnectors {
	t.Helper()
	connectors, err := adapter.NewAccountingConnectors(db)
	if err != nil {
		t.Fatal(err)
	}
	return connectors
}

func accountingConnectorRegistration(
	t *testing.T,
	kind domain.AccountingExchangeKind,
	counterparty string,
) domain.AccountingConnectorRegistration {
	t.Helper()
	reference, err := domain.NewAccountingCounterpartyReference(counterparty)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewAccountingConnectorRegistration(kind, reference, domain.AccountingConnectorCanonical)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
