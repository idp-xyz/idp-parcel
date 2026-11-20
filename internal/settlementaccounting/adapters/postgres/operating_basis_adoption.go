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

// OperatingBasisAdoptions 是一户一行的口径采用册。
type OperatingBasisAdoptions struct {
	db *bentopg.DB
}

func NewOperatingBasisAdoptions(db *bentopg.DB) (*OperatingBasisAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &OperatingBasisAdoptions{db: db}, nil
}

func (repository *OperatingBasisAdoptions) FindOperatingBasisAdoption(
	ctx context.Context,
	tenant domain.TenantID,
) (domain.OperatingBasisAdoption, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OperatingBasisAdoption{}, false, fmt.Errorf("find operating basis adoption: %w", err)
	}
	var estimated, confirmed bool
	err = querier.QueryRow(ctx,
		`SELECT adopt_estimated, adopt_confirmed
		   FROM settlement_accounting.operating_basis_adoption
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&estimated, &confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OperatingBasisAdoption{}, false, nil
	}
	if err != nil {
		return domain.OperatingBasisAdoption{}, false, fmt.Errorf("find operating basis adoption: %w", err)
	}
	adoption, err := domain.NewOperatingBasisAdoption(estimated, confirmed)
	if err != nil {
		return domain.OperatingBasisAdoption{}, false, fmt.Errorf("find operating basis adoption: %w", err)
	}
	return adoption, true, nil
}

func (repository *OperatingBasisAdoptions) SaveOperatingBasisAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	adoption domain.OperatingBasisAdoption,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save operating basis adoption: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.operating_basis_adoption
			(tenant_id, adopt_estimated, adopt_confirmed, recorded_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), adoption.Estimated(), adoption.Confirmed(), at.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("save operating basis adoption: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.CatalogueRegistered, nil
	}
	existing, found, err := repository.FindOperatingBasisAdoption(ctx, tenant)
	if err != nil {
		return 0, fmt.Errorf("save operating basis adoption: %w", err)
	}
	if !found {
		return 0, fmt.Errorf("save operating basis adoption: conflict produced no row")
	}
	if existing.Equal(adoption) {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}
