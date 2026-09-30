package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PeriodicFees 是周期费用形态登记册。空册答 found=false。算法不在行上。
type PeriodicFees struct {
	db *bentopg.DB
}

func NewPeriodicFees(db *bentopg.DB) (*PeriodicFees, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PeriodicFees{db: db}, nil
}

func (repository *PeriodicFees) SavePeriodicFee(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.PeriodicFeeRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save periodic fee: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT periodic_fee_form"); err != nil {
		return 0, fmt.Errorf("save periodic fee: %w", err)
	}
	terms := registration.Terms()
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.periodic_fee_form (
			tenant_id, rule_ref, form_kind, minimum_minor, committed_quantity, rate_minor, registered_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenant.String(),
		registration.Rule().String(),
		terms.Form().String(),
		periodicMinimum(terms),
		periodicCommitted(terms),
		periodicRate(terms),
		at.UTC(),
	)
	if uniqueViolation(err) {
		if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT periodic_fee_form"); rollbackErr != nil {
			return 0, fmt.Errorf("save periodic fee: %w", rollbackErr)
		}
		loaded, found, loadErr := loadPeriodicFee(ctx, executor, tenant, registration.Rule())
		if loadErr != nil {
			return 0, fmt.Errorf("save periodic fee: %w", loadErr)
		}
		if !found || !registration.Terms().Same(loaded) {
			return ports.CatalogueConflict, nil
		}
		return ports.CatalogueReplay, nil
	}
	if err != nil {
		return 0, fmt.Errorf("save periodic fee: %w", err)
	}
	if err := insertPeriodicTiers(ctx, executor, tenant, registration); err != nil {
		return 0, fmt.Errorf("save periodic fee: %w", err)
	}
	return ports.CatalogueRegistered, nil
}

func (repository *PeriodicFees) LoadPeriodicFee(
	ctx context.Context,
	tenant domain.TenantID,
	rule domain.PeriodicFeeRuleReference,
) (domain.PeriodicFeeTerms, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PeriodicFeeTerms{}, false, fmt.Errorf("load periodic fee: %w", err)
	}
	return loadPeriodicFee(ctx, querier, tenant, rule)
}

type periodicReader interface {
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

type periodicWriter interface {
	periodicReader
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertPeriodicTiers(
	ctx context.Context,
	executor periodicWriter,
	tenant domain.TenantID,
	registration domain.PeriodicFeeRegistration,
) error {
	if registration.Terms().Form() != domain.TieredRebate {
		return nil
	}
	for index, tier := range registration.Terms().Tiers() {
		var upper *int64
		if !tier.Open() {
			value := tier.UpperMinor()
			upper = &value
		}
		if _, err := executor.Exec(ctx, `
			INSERT INTO settlement_accounting.periodic_fee_tier (
				tenant_id, rule_ref, tier_ordinal, upper_minor, rate_basis_points
			) VALUES ($1, $2, $3, $4, $5)`,
			tenant.String(), registration.Rule().String(), index+1, upper, tier.RateBasisPoints()); err != nil {
			return err
		}
	}
	return nil
}

func loadPeriodicFee(
	ctx context.Context,
	querier periodicReader,
	tenant domain.TenantID,
	rule domain.PeriodicFeeRuleReference,
) (domain.PeriodicFeeTerms, bool, error) {
	var kind string
	var minimum, committed, rate *int64
	err := querier.QueryRow(ctx, `
		SELECT form_kind, minimum_minor, committed_quantity, rate_minor
		  FROM settlement_accounting.periodic_fee_form
		 WHERE tenant_id = $1 AND rule_ref = $2`,
		tenant.String(), rule.String(),
	).Scan(&kind, &minimum, &committed, &rate)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PeriodicFeeTerms{}, false, nil
	}
	if err != nil {
		return domain.PeriodicFeeTerms{}, false, err
	}
	form, err := domain.PeriodicFeeFormFromName(kind)
	if err != nil {
		return domain.PeriodicFeeTerms{}, false, err
	}
	switch form {
	case domain.MinimumSpend:
		if minimum == nil {
			return domain.PeriodicFeeTerms{}, false, fmt.Errorf("periodic fee: minimum missing")
		}
		terms, err := domain.NewMinimumSpendTerms(*minimum)
		return terms, err == nil, err
	case domain.VolumeFloor:
		if committed == nil || rate == nil {
			return domain.PeriodicFeeTerms{}, false, fmt.Errorf("periodic fee: volume floor missing")
		}
		terms, err := domain.NewVolumeFloorTerms(*committed, *rate)
		return terms, err == nil, err
	case domain.PeriodicFeeNotApplicable:
		return domain.NewPeriodicFeeNotApplicable(), true, nil
	case domain.TieredRebate:
		tiers, err := loadPeriodicTiers(ctx, querier, tenant, rule)
		if err != nil {
			return domain.PeriodicFeeTerms{}, false, err
		}
		terms, err := domain.NewTieredRebateTerms(tiers)
		return terms, err == nil, err
	default:
		return domain.PeriodicFeeTerms{}, false, fmt.Errorf("periodic fee: %w", domain.ErrInvalidPeriodicFee)
	}
}

func loadPeriodicTiers(
	ctx context.Context,
	querier periodicReader,
	tenant domain.TenantID,
	rule domain.PeriodicFeeRuleReference,
) ([]domain.RebateTier, error) {
	rows, err := querier.Query(ctx, `
		SELECT upper_minor, rate_basis_points
		  FROM settlement_accounting.periodic_fee_tier
		 WHERE tenant_id = $1 AND rule_ref = $2
		 ORDER BY tier_ordinal`,
		tenant.String(), rule.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tiers []domain.RebateTier
	for rows.Next() {
		var upper *int64
		var rate int64
		if err := rows.Scan(&upper, &rate); err != nil {
			return nil, err
		}
		var tier domain.RebateTier
		if upper == nil {
			tier, err = domain.NewOpenRebateTier(rate)
		} else {
			tier, err = domain.NewRebateTier(*upper, rate)
		}
		if err != nil {
			return nil, err
		}
		tiers = append(tiers, tier)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tiers, nil
}

func periodicMinimum(terms domain.PeriodicFeeTerms) *int64 {
	if terms.Form() != domain.MinimumSpend {
		return nil
	}
	value := terms.MinimumMinor()
	return &value
}

func periodicCommitted(terms domain.PeriodicFeeTerms) *int64 {
	if terms.Form() != domain.VolumeFloor {
		return nil
	}
	value := terms.CommittedQuantity()
	return &value
}

func periodicRate(terms domain.PeriodicFeeTerms) *int64 {
	if terms.Form() != domain.VolumeFloor {
		return nil
	}
	value := terms.RateMinor()
	return &value
}
