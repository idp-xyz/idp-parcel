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

// FeeItemMappings 是费用项目映射册（ADR-0190 决定五）。写入在环境事务里；读口可以在事务外。
type FeeItemMappings struct {
	db *bentopg.DB
}

func NewFeeItemMappings(db *bentopg.DB) (*FeeItemMappings, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &FeeItemMappings{db: db}, nil
}

var (
	_ ports.FeeItemMappingRegister = (*FeeItemMappings)(nil)
	_ ports.FeeItemMappingView     = (*FeeItemMappings)(nil)
)

// SaveFeeItemMapping 插一行；主键撞上即读回同一代码比内容，分重放与冲突，不覆盖。保存点的理由同
// SellBillingMomentAdoptions.SaveSellBillingMomentAdoption。
func (repository *FeeItemMappings) SaveFeeItemMapping(
	ctx context.Context,
	tenant domain.TenantID,
	mapping domain.FeeItemMapping,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save fee item mapping: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT fee_item_mapping_insert"); err != nil {
		return 0, fmt.Errorf("save fee item mapping: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.fee_item_mapping (
			tenant_id, pricing_charge_code, fee_item, basis_ref, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		mapping.ChargeCode().String(),
		mapping.FeeItem().String(),
		mapping.Basis().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save fee item mapping: %w", err)
	}
	if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT fee_item_mapping_insert"); rollbackErr != nil {
		return 0, fmt.Errorf("save fee item mapping: %w", rollbackErr)
	}
	stored, found, err := findFeeItemMapping(ctx, executor, tenant, mapping.ChargeCode())
	if err != nil {
		return 0, fmt.Errorf("save fee item mapping: %w", err)
	}
	if found && stored.SameRegistration(mapping) {
		return ports.CatalogueReplay, nil
	}
	return ports.CatalogueConflict, nil
}

func (repository *FeeItemMappings) FindFeeItemMapping(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.PricingChargeCode,
) (domain.FeeItemMapping, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.FeeItemMapping{}, false, fmt.Errorf("find fee item mapping: %w", err)
	}
	mapping, found, err := findFeeItemMapping(ctx, querier, tenant, code)
	if err != nil {
		return domain.FeeItemMapping{}, false, fmt.Errorf("find fee item mapping: %w", err)
	}
	return mapping, found, nil
}

func findFeeItemMapping(
	ctx context.Context,
	querier interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	},
	tenant domain.TenantID,
	code domain.PricingChargeCode,
) (domain.FeeItemMapping, bool, error) {
	var storedCode, feeItem, basis string
	err := querier.QueryRow(ctx, `
		SELECT pricing_charge_code, fee_item, basis_ref
		  FROM settlement_accounting.fee_item_mapping
		 WHERE tenant_id = $1 AND pricing_charge_code = $2`,
		tenant.String(), code.String(),
	).Scan(&storedCode, &feeItem, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FeeItemMapping{}, false, nil
	}
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	parsedCode, err := domain.NewPricingChargeCode(storedCode)
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	parsedFeeItem, err := domain.NewFeeItemReference(feeItem)
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	parsedBasis, err := domain.NewFeeItemMappingBasisReference(basis)
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	mapping, err := domain.NewFeeItemMapping(parsedCode, parsedFeeItem, parsedBasis)
	if err != nil {
		return domain.FeeItemMapping{}, false, err
	}
	return mapping, true, nil
}
