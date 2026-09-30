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

// ChargeAttributions 是费用项目选用哪套归属日判定的登记册。空册答 found=false。
type ChargeAttributions struct {
	db *bentopg.DB
}

func NewChargeAttributions(db *bentopg.DB) (*ChargeAttributions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ChargeAttributions{db: db}, nil
}

var (
	_ ports.ChargeAttributionRegister = (*ChargeAttributions)(nil)
	_ ports.ChargeAttributionView     = (*ChargeAttributions)(nil)
)

func (repository *ChargeAttributions) SaveChargeAttribution(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.ChargeAttributionRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save charge attribution: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT charge_attribution"); err != nil {
		return 0, fmt.Errorf("save charge attribution: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.charge_attribution (
			tenant_id, fee_item, form, timezone_name, cutoff_minute, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(),
		registration.FeeItem().String(),
		registration.Form().String(),
		registration.TimeZone(),
		registration.CutoffMinute(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save charge attribution: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT charge_attribution"); err != nil {
		return 0, fmt.Errorf("save charge attribution: %w", err)
	}
	loaded, found, err := scanChargeAttribution(executor.QueryRow(ctx, `
		SELECT form, timezone_name, cutoff_minute
		  FROM settlement_accounting.charge_attribution
		 WHERE tenant_id = $1 AND fee_item = $2`,
		tenant.String(), registration.FeeItem().String()), registration.FeeItem())
	if err != nil {
		return 0, fmt.Errorf("save charge attribution: %w", err)
	}
	if !found || !registration.SameRegistration(loaded) {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *ChargeAttributions) LoadChargeAttribution(
	ctx context.Context,
	tenant domain.TenantID,
	feeItem domain.FeeItemReference,
) (domain.ChargeAttributionRegistration, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ChargeAttributionRegistration{}, false, fmt.Errorf("load charge attribution: %w", err)
	}
	registration, found, err := scanChargeAttribution(querier.QueryRow(ctx, `
		SELECT form, timezone_name, cutoff_minute
		  FROM settlement_accounting.charge_attribution
		 WHERE tenant_id = $1 AND fee_item = $2`,
		tenant.String(), feeItem.String()), feeItem)
	if err != nil {
		return domain.ChargeAttributionRegistration{}, false, fmt.Errorf("load charge attribution: %w", err)
	}
	return registration, found, nil
}

func scanChargeAttribution(row pgx.Row, feeItem domain.FeeItemReference) (domain.ChargeAttributionRegistration, bool, error) {
	var formName, zone string
	var cutoff int
	err := row.Scan(&formName, &zone, &cutoff)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChargeAttributionRegistration{}, false, nil
	}
	if err != nil {
		return domain.ChargeAttributionRegistration{}, false, err
	}
	form, err := domain.ChargeAttributionFormFromName(formName)
	if err != nil {
		return domain.ChargeAttributionRegistration{}, false, err
	}
	registration, err := domain.NewChargeAttributionRegistration(feeItem, form, zone, cutoff)
	if err != nil {
		return domain.ChargeAttributionRegistration{}, false, err
	}
	return registration, true, nil
}
