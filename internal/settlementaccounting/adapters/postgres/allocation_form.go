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

// AllocationForms 是分摊规则版本选用哪套分法的登记册。空册答 found=false。
// 展开不在行上。各对象的权重也不在行上。
type AllocationForms struct {
	db *bentopg.DB
}

func NewAllocationForms(db *bentopg.DB) (*AllocationForms, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AllocationForms{db: db}, nil
}

var (
	_ ports.AllocationFormRegister = (*AllocationForms)(nil)
	_ ports.AllocationFormView     = (*AllocationForms)(nil)
)

func (repository *AllocationForms) SaveAllocationForm(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.AllocationFormRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save allocation form: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT allocation_form_choice"); err != nil {
		return 0, fmt.Errorf("save allocation form: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.allocation_form_choice (
			tenant_id, rule_version, form, registered_at
		) VALUES ($1, $2, $3, $4)`,
		tenant.String(),
		registration.Rule().String(),
		registration.Form().String(),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save allocation form: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT allocation_form_choice"); err != nil {
		return 0, fmt.Errorf("save allocation form: %w", err)
	}
	loaded, found, err := scanAllocationForm(executor.QueryRow(ctx, `
		SELECT form
		  FROM settlement_accounting.allocation_form_choice
		 WHERE tenant_id = $1 AND rule_version = $2`,
		tenant.String(), registration.Rule().String()))
	if err != nil {
		return 0, fmt.Errorf("save allocation form: %w", err)
	}
	if !found || loaded != registration.Form() {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *AllocationForms) LoadAllocationForm(
	ctx context.Context,
	tenant domain.TenantID,
	rule domain.AllocationRuleVersionReference,
) (domain.AllocationForm, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.AllocationFormInvalid, false, fmt.Errorf("load allocation form: %w", err)
	}
	form, found, err := scanAllocationForm(querier.QueryRow(ctx, `
		SELECT form
		  FROM settlement_accounting.allocation_form_choice
		 WHERE tenant_id = $1 AND rule_version = $2`,
		tenant.String(), rule.String()))
	if err != nil {
		return domain.AllocationFormInvalid, false, fmt.Errorf("load allocation form: %w", err)
	}
	return form, found, nil
}

func scanAllocationForm(row pgx.Row) (domain.AllocationForm, bool, error) {
	var name string
	err := row.Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AllocationFormInvalid, false, nil
	}
	if err != nil {
		return domain.AllocationFormInvalid, false, err
	}
	form, err := domain.AllocationFormFromName(name)
	if err != nil {
		return domain.AllocationFormInvalid, false, err
	}
	return form, true, nil
}
