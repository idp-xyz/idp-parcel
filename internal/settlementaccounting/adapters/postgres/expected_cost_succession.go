package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var _ ports.ExpectedCostSuccessionView = (*SupplierExpectedCosts)(nil)

// LoadExpectedCostSuccessor 交回指这一版的那一版；一版至多被纠正一次（迁移 SA 0044 的唯一索引），所以至多一行。
func (repository *SupplierExpectedCosts) LoadExpectedCostSuccessor(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SupplierCostVersionID,
) (domain.SupplierCostVersionID, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SupplierCostVersionID{}, false, fmt.Errorf("load expected cost successor: %w", err)
	}
	var raw string
	err = querier.QueryRow(ctx,
		`SELECT version FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1 AND prior_version = $2`,
		tenant.String(), version.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SupplierCostVersionID{}, false, nil
	}
	if err != nil {
		return domain.SupplierCostVersionID{}, false, fmt.Errorf("load expected cost successor: %w", err)
	}
	successor, err := domain.NewSupplierCostVersionID(raw)
	if err != nil {
		return domain.SupplierCostVersionID{}, false, fmt.Errorf("load expected cost successor: %w", err)
	}
	return successor, true, nil
}
