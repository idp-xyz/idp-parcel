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

// SettlementMoments 是租户采用了确认或截单的登记册。空册答 found=false。不存钟点，不存账期。
type SettlementMoments struct {
	db *bentopg.DB
}

func NewSettlementMoments(db *bentopg.DB) (*SettlementMoments, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SettlementMoments{db: db}, nil
}

var (
	_ ports.SettlementMomentRegister = (*SettlementMoments)(nil)
	_ ports.SettlementMomentView     = (*SettlementMoments)(nil)
)

func (repository *SettlementMoments) SaveSettlementMoment(
	ctx context.Context,
	tenant domain.TenantID,
	moment domain.SettlementMoment,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save settlement moment: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT settlement_moment"); err != nil {
		return 0, fmt.Errorf("save settlement moment: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.settlement_moment (
			tenant_id, moment, registered_at
		) VALUES ($1, $2, $3)`,
		tenant.String(), moment.String(), at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save settlement moment: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT settlement_moment"); err != nil {
		return 0, fmt.Errorf("save settlement moment: %w", err)
	}
	found, err := repository.momentExists(ctx, executor, tenant, moment)
	if err != nil {
		return 0, fmt.Errorf("save settlement moment: %w", err)
	}
	if !found {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *SettlementMoments) LoadSettlementMoment(
	ctx context.Context,
	tenant domain.TenantID,
	moment domain.SettlementMoment,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load settlement moment: %w", err)
	}
	found, err := repository.momentExists(ctx, querier, tenant, moment)
	if err != nil {
		return false, fmt.Errorf("load settlement moment: %w", err)
	}
	return found, nil
}

type momentQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (repository *SettlementMoments) momentExists(
	ctx context.Context,
	querier momentQuerier,
	tenant domain.TenantID,
	moment domain.SettlementMoment,
) (bool, error) {
	var stored string
	err := querier.QueryRow(ctx, `
		SELECT moment
		  FROM settlement_accounting.settlement_moment
		 WHERE tenant_id = $1 AND moment = $2`,
		tenant.String(), moment.String(),
	).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
