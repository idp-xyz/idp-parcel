package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// DirectOperatingDispositions 记下答不出毛利的当前答案，并在写下这种答案时撤下同一范围同一口径的经营结果。
type DirectOperatingDispositions struct {
	db *bentopg.DB
}

func NewDirectOperatingDispositions(db *bentopg.DB) (*DirectOperatingDispositions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &DirectOperatingDispositions{db: db}, nil
}

func (repository *DirectOperatingDispositions) SaveDirectOperatingDisposition(
	ctx context.Context,
	tenant domain.TenantID,
	scope string,
	basis domain.OperatingBasis,
	disposition string,
	at time.Time,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save direct operating disposition: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.direct_operating_disposition
			(tenant_id, scope_ref, basis, disposition, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, scope_ref, basis) DO UPDATE
		    SET disposition = EXCLUDED.disposition,
		        recorded_at = EXCLUDED.recorded_at`,
		tenant.String(), scope, basis.String(), disposition, at.UTC(),
	); err != nil {
		return fmt.Errorf("save direct operating disposition: %w", err)
	}
	return repository.deleteResults(ctx, tenant, scope, basis)
}

func (repository *DirectOperatingDispositions) ClearDirectOperatingDisposition(
	ctx context.Context,
	tenant domain.TenantID,
	scope string,
	basis domain.OperatingBasis,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("clear direct operating disposition: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`DELETE FROM settlement_accounting.direct_operating_disposition
		  WHERE tenant_id = $1 AND scope_ref = $2 AND basis = $3`,
		tenant.String(), scope, basis.String(),
	); err != nil {
		return fmt.Errorf("clear direct operating disposition: %w", err)
	}
	return nil
}

func (repository *DirectOperatingDispositions) ForgetDirectOperatingAnswer(
	ctx context.Context,
	tenant domain.TenantID,
	scope string,
	basis domain.OperatingBasis,
) error {
	if err := repository.ClearDirectOperatingDisposition(ctx, tenant, scope, basis); err != nil {
		return err
	}
	return repository.deleteResults(ctx, tenant, scope, basis)
}

func (repository *DirectOperatingDispositions) deleteResults(
	ctx context.Context,
	tenant domain.TenantID,
	scope string,
	basis domain.OperatingBasis,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("drop operating result: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`DELETE FROM settlement_accounting.operating_result
		  WHERE tenant_id = $1 AND scope_ref = $2 AND basis = $3`,
		tenant.String(), scope, basis.String(),
	); err != nil {
		return fmt.Errorf("drop operating result: %w", err)
	}
	return nil
}

var _ ports.DirectOperatingDispositionStore = (*DirectOperatingDispositions)(nil)
