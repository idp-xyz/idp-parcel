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

// ShipmentBillingMomentAdoptions 是委托级计费时点采用册。写入在环境事务里；读口可以在事务外。不插默认行。
type ShipmentBillingMomentAdoptions struct {
	db *bentopg.DB
}

func NewShipmentBillingMomentAdoptions(db *bentopg.DB) (*ShipmentBillingMomentAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ShipmentBillingMomentAdoptions{db: db}, nil
}

var (
	_ ports.ShipmentBillingMomentAdoptionRegister = (*ShipmentBillingMomentAdoptions)(nil)
	_ ports.ShipmentBillingMomentAdoptionView     = (*ShipmentBillingMomentAdoptions)(nil)
)

func (repository *ShipmentBillingMomentAdoptions) SaveShipmentBillingMomentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	adoption domain.ShipmentBillingMomentAdoption,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT shipment_billing_moment_adoption_insert"); err != nil {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.shipment_billing_moment_adoption (
			tenant_id, account_id, billing_moment, pricing_scope_id, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		adoption.Account().String(),
		adoption.Moment().String(),
		optionalScope(adoption.PricingScope().String()),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	missing, duplicate := foreignKeyViolation(err), uniqueViolation(err)
	if !missing && !duplicate {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", err)
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT shipment_billing_moment_adoption_insert"); rollbackErr != nil {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", rollbackErr)
	}
	if missing {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", domain.ErrCatalogueTargetMissing)
	}
	stored, found, err := findShipmentBillingMomentAdoption(ctx, executor, tenant, adoption.Account())
	if err != nil {
		return 0, fmt.Errorf("save shipment billing moment adoption: %w", err)
	}
	if found && stored.SameRegistration(adoption) {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}

func (repository *ShipmentBillingMomentAdoptions) FindShipmentBillingMomentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.ShipmentBillingMomentAdoption, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, fmt.Errorf("find shipment billing moment adoption: %w", err)
	}
	adoption, found, err := findShipmentBillingMomentAdoption(ctx, querier, tenant, account)
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, fmt.Errorf("find shipment billing moment adoption: %w", err)
	}
	return adoption, found, nil
}

func findShipmentBillingMomentAdoption(
	ctx context.Context,
	querier interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.ShipmentBillingMomentAdoption, bool, error) {
	var accountID, moment string
	var scope *string
	err := querier.QueryRow(ctx, `
		SELECT account_id, billing_moment, pricing_scope_id
		  FROM settlement_accounting.shipment_billing_moment_adoption
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.String(),
	).Scan(&accountID, &moment, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShipmentBillingMomentAdoption{}, false, nil
	}
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, err
	}
	storedAccount, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, err
	}
	storedMoment, err := domain.ShipmentBillingMomentFromName(moment)
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, err
	}
	var storedScope domain.EstimatePricingScope
	if scope != nil {
		storedScope, err = domain.NewEstimatePricingScope(*scope)
		if err != nil {
			return domain.ShipmentBillingMomentAdoption{}, false, err
		}
	}
	adoption, err := domain.NewShipmentBillingMomentAdoption(storedAccount, storedMoment, storedScope)
	if err != nil {
		return domain.ShipmentBillingMomentAdoption{}, false, err
	}
	return adoption, true, nil
}
