package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AccountingConnectors 是某个对方的一种交换选用哪种连接器形态的登记册。空册答 found=false。
type AccountingConnectors struct {
	db *bentopg.DB
}

func NewAccountingConnectors(db *bentopg.DB) (*AccountingConnectors, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AccountingConnectors{db: db}, nil
}

var (
	_ ports.AccountingConnectorRegister = (*AccountingConnectors)(nil)
	_ ports.AccountingConnectorView     = (*AccountingConnectors)(nil)
)

func (repository *AccountingConnectors) SaveAccountingConnector(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AccountingConnectorRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save accounting connector: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT accounting_connector"); err != nil {
		return 0, fmt.Errorf("save accounting connector: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.accounting_connector (
			tenant_id, exchange_kind, counterparty, form, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		registration.Kind().String(),
		registration.Counterparty().String(),
		registration.Form().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save accounting connector: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT accounting_connector"); err != nil {
		return 0, fmt.Errorf("save accounting connector: %w", err)
	}
	loaded, found, err := scanAccountingConnector(executor.QueryRow(ctx, `
		SELECT form
		  FROM settlement_accounting.accounting_connector
		 WHERE tenant_id = $1 AND exchange_kind = $2 AND counterparty = $3`,
		tenant.String(), registration.Kind().String(), registration.Counterparty().String()))
	if err != nil {
		return 0, fmt.Errorf("save accounting connector: %w", err)
	}
	if !found || loaded != registration.Form() {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *AccountingConnectors) LoadAccountingConnector(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.AccountingExchangeKind,
	counterparty domain.AccountingCounterpartyReference,
) (domain.AccountingConnectorForm, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.AccountingConnectorFormInvalid, false, fmt.Errorf("load accounting connector: %w", err)
	}
	form, found, err := scanAccountingConnector(querier.QueryRow(ctx, `
		SELECT form
		  FROM settlement_accounting.accounting_connector
		 WHERE tenant_id = $1 AND exchange_kind = $2 AND counterparty = $3`,
		tenant.String(), kind.String(), counterparty.String()))
	if err != nil {
		return domain.AccountingConnectorFormInvalid, false, fmt.Errorf("load accounting connector: %w", err)
	}
	return form, found, nil
}

func scanAccountingConnector(row pgx.Row) (domain.AccountingConnectorForm, bool, error) {
	var name string
	err := row.Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AccountingConnectorFormInvalid, false, nil
	}
	if err != nil {
		return domain.AccountingConnectorFormInvalid, false, err
	}
	form, err := domain.AccountingConnectorFormFromName(name)
	if err != nil {
		return domain.AccountingConnectorFormInvalid, false, err
	}
	return form, true, nil
}
