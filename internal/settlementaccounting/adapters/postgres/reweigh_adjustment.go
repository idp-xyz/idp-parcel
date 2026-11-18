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

// ChargeSupersessions 是后一代预估替代前一代未确认费用的登记册。
type ChargeSupersessions struct {
	db *bentopg.DB
}

func NewChargeSupersessions(db *bentopg.DB) (*ChargeSupersessions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ChargeSupersessions{db: db}, nil
}

var _ ports.ChargeSupersessionStore = (*ChargeSupersessions)(nil)

func (repository *ChargeSupersessions) SaveSupersession(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
	supersededBy domain.CustomerChargeID,
	at time.Time,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save charge supersession: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.customer_charge_supersession (
			tenant_id, charge_id, superseded_by_charge_id, recorded_at
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT customer_charge_supersession_pkey DO NOTHING`,
		tenant.String(), charge.String(), supersededBy.String(), at.UTC(),
	)
	if err != nil {
		return fmt.Errorf("save charge supersession: %w", err)
	}
	return nil
}

func (repository *ChargeSupersessions) Superseded(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load charge supersession: %w", err)
	}
	var stored string
	err = querier.QueryRow(ctx, `
		SELECT charge_id
		  FROM settlement_accounting.customer_charge_supersession
		 WHERE tenant_id = $1 AND charge_id = $2`,
		tenant.String(), charge.String(),
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load charge supersession: %w", err)
	}
	return true, nil
}

// ReweighAdjustmentAdoptions 是结算账户采用「实测差异自动调整」的登记册。
type ReweighAdjustmentAdoptions struct {
	db *bentopg.DB
}

func NewReweighAdjustmentAdoptions(db *bentopg.DB) (*ReweighAdjustmentAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ReweighAdjustmentAdoptions{db: db}, nil
}

var (
	_ ports.ReweighAdjustmentAdoptionView     = (*ReweighAdjustmentAdoptions)(nil)
	_ ports.ReweighAdjustmentAdoptionRegister = (*ReweighAdjustmentAdoptions)(nil)
)

func (repository *ReweighAdjustmentAdoptions) SaveReweighAdjustmentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
	basis string,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save reweigh adjustment adoption: %w", err)
	}
	tag, err := executor.Exec(ctx, `
		INSERT INTO settlement_accounting.reweigh_adjustment_adoption (
			tenant_id, account_id, basis, recorded_at
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT ON CONSTRAINT reweigh_adjustment_adoption_pkey DO NOTHING`,
		tenant.String(), account.String(), basis, at.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save reweigh adjustment adoption: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.CatalogueRegistered, nil
	}
	var stored string
	err = executor.QueryRow(ctx, `
		SELECT basis
		  FROM settlement_accounting.reweigh_adjustment_adoption
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.String(),
	).Scan(&stored)
	if err != nil {
		return 0, fmt.Errorf("save reweigh adjustment adoption: %w", err)
	}
	if stored == basis {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}

func (repository *ReweighAdjustmentAdoptions) ReweighAdjustmentAdopted(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load reweigh adjustment adoption: %w", err)
	}
	var stored string
	err = querier.QueryRow(ctx, `
		SELECT account_id
		  FROM settlement_accounting.reweigh_adjustment_adoption
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.String(),
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load reweigh adjustment adoption: %w", err)
	}
	return true, nil
}
