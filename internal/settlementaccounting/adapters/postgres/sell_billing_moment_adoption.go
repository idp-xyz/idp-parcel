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

// SellBillingMomentAdoptions 是 SELL 计费时点采用册（ADR-0190 决定三）。写入在环境事务里；读口可以在事务外。
type SellBillingMomentAdoptions struct {
	db *bentopg.DB
}

func NewSellBillingMomentAdoptions(db *bentopg.DB) (*SellBillingMomentAdoptions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SellBillingMomentAdoptions{db: db}, nil
}

var (
	_ ports.SellBillingMomentAdoptionRegister = (*SellBillingMomentAdoptions)(nil)
	_ ports.SellBillingMomentAdoptionView     = (*SellBillingMomentAdoptions)(nil)
)

// SaveSellBillingMomentAdoption 插一行；主键撞上即读回同一户比内容，分重放与冲突，不覆盖。账户不在册上由外键拒，
// 译成 ErrCatalogueTargetMissing。先设保存点：撞约束会让整笔事务进入中止态，不退回保存点就读不回已在册的那一行。
func (repository *SellBillingMomentAdoptions) SaveSellBillingMomentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	adoption domain.SellBillingMomentAdoption,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT sell_billing_moment_adoption_insert"); err != nil {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.sell_billing_moment_adoption (
			tenant_id, account_id, billing_moment, pricing_scope_id, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		adoption.Account().String(),
		adoption.Moment().String(),
		adoption.PricingScope().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	missing, duplicate := foreignKeyViolation(err), uniqueViolation(err)
	if !missing && !duplicate {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", err)
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT sell_billing_moment_adoption_insert"); rollbackErr != nil {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", rollbackErr)
	}
	if missing {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", domain.ErrCatalogueTargetMissing)
	}
	stored, found, err := findSellBillingMomentAdoption(ctx, executor, tenant, adoption.Account())
	if err != nil {
		return 0, fmt.Errorf("save sell billing moment adoption: %w", err)
	}
	if found && stored.SameRegistration(adoption) {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}

func (repository *SellBillingMomentAdoptions) FindSellBillingMomentAdoption(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.SellBillingMomentAdoption, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, fmt.Errorf("find sell billing moment adoption: %w", err)
	}
	adoption, found, err := findSellBillingMomentAdoption(ctx, querier, tenant, account)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, fmt.Errorf("find sell billing moment adoption: %w", err)
	}
	return adoption, found, nil
}

func findSellBillingMomentAdoption(
	ctx context.Context,
	querier interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (domain.SellBillingMomentAdoption, bool, error) {
	var accountID, moment, scope string
	err := querier.QueryRow(ctx, `
		SELECT account_id, billing_moment, pricing_scope_id
		  FROM settlement_accounting.sell_billing_moment_adoption
		 WHERE tenant_id = $1 AND account_id = $2`,
		tenant.String(), account.String(),
	).Scan(&accountID, &moment, &scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SellBillingMomentAdoption{}, false, nil
	}
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, err
	}
	storedAccount, err := domain.NewSettlementAccountID(accountID)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, err
	}
	storedMoment, err := domain.ParcelBillingMomentFromName(moment)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, err
	}
	storedScope, err := domain.NewEstimatePricingScope(scope)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, err
	}
	adoption, err := domain.NewSellBillingMomentAdoption(storedAccount, storedMoment, storedScope)
	if err != nil {
		return domain.SellBillingMomentAdoption{}, false, err
	}
	return adoption, true, nil
}
