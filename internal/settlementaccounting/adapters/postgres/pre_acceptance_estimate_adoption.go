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

// PreAcceptanceEstimateAdoptions 是受理前估价采用册（ADR-0179 决定五）。写入在环境事务里；读口可以在事务外。
type PreAcceptanceEstimateAdoptions struct {
	db *bentopg.DB
}

func NewPreAcceptanceEstimateAdoptions(db *bentopg.DB) (*PreAcceptanceEstimateAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PreAcceptanceEstimateAdoptions{db: db}, nil
}

var (
	_ ports.PreAcceptanceEstimateAdoptionRegister = (*PreAcceptanceEstimateAdoptions)(nil)
	_ ports.PreAcceptanceEstimateAdoptionView     = (*PreAcceptanceEstimateAdoptions)(nil)
)

// SaveAdoption 插一行；主键撞上即读回同一户比内容，分重放与冲突，不覆盖。账户不在册上由外键拒，译成 ErrCatalogueTargetMissing。
func (repository *PreAcceptanceEstimateAdoptions) SaveAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	adoption domain.PreAcceptanceEstimateAdoption,
	at time.Time,
) (ports.PreAcceptanceEstimateAdoptionEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT pre_acceptance_estimate_adoption_insert"); err != nil {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.pre_acceptance_estimate_adoption (
			tenant_id, account_id, form, pricing_scope_id, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		adoption.Account().String(),
		adoption.Form().String(),
		adoption.PricingScope().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.PreAcceptanceEstimateAdoptionRegistered, nil
	}
	missing, duplicate := foreignKeyViolation(err), uniqueViolation(err)
	if !missing && !duplicate {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", err)
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT pre_acceptance_estimate_adoption_insert"); rollbackErr != nil {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", rollbackErr)
	}
	if missing {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", domain.ErrCatalogueTargetMissing)
	}
	stored, found, err := repository.find(ctx, executor, tenant, adoption.Account())
	if err != nil {
		return 0, fmt.Errorf("save pre-acceptance estimate adoption: %w", err)
	}
	if found && stored.SameRegistration(adoption) {
		return ports.PreAcceptanceEstimateAdoptionReplay, nil
	}
	return ports.PreAcceptanceEstimateAdoptionConflict, nil
}

func (repository *PreAcceptanceEstimateAdoptions) FindAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.PreAcceptanceEstimateAdoption, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, fmt.Errorf("find pre-acceptance estimate adoption: %w", err)
	}
	adoption, found, err := repository.find(ctx, querier, tenant, account)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, fmt.Errorf("find pre-acceptance estimate adoption: %w", err)
	}
	return adoption, found, nil
}

func (repository *PreAcceptanceEstimateAdoptions) find(
	ctx context.Context,
	querier interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.PreAcceptanceEstimateAdoption, bool, error) {
	var accountID, form, scope string
	err := querier.QueryRow(ctx, `
		SELECT account_id, form, pricing_scope_id
		  FROM settlement_accounting.pre_acceptance_estimate_adoption
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.String(),
	).Scan(&accountID, &form, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PreAcceptanceEstimateAdoption{}, false, nil
	}
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, err
	}
	storedAccount, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, err
	}
	storedForm, err := domain.PreAcceptanceEstimateFormFromName(form)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, err
	}
	storedScope, err := domain.NewEstimatePricingScope(scope)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, err
	}
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(storedAccount, storedForm, storedScope)
	if err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, err
	}
	return adoption, true, nil
}
