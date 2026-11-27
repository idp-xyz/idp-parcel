package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 服务产品名称与范围叫法。不进内容摘要：摘要盖正文，名称是这一版给人看的标签，
// 范围叫法是那串范围字的名册。同内容再写是重放，换叫法不覆盖。

func (catalogue *OperationsCatalogue) ListScopeNames(ctx context.Context, tenant domain.TenantID) ([]ports.CommercialScopeName, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scope names: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT scope_ref, name
		   FROM party_commercial.commercial_scope_name
		  WHERE tenant_id = $1
		  ORDER BY scope_ref`,
		tenant.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list scope names: %w", err)
	}
	defer rows.Close()
	names := []ports.CommercialScopeName{}
	for rows.Next() {
		var name ports.CommercialScopeName
		if err := rows.Scan(&name.Scope, &name.Name); err != nil {
			return nil, fmt.Errorf("list scope names: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (catalogue *OperationsCatalogue) SaveScopeName(ctx context.Context, tenant domain.TenantID, scope, name string) (ports.LabelSaveOutcome, error) {
	var outcome ports.LabelSaveOutcome
	err := catalogue.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := catalogue.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		saved, err := insertLabel(txCtx, executor,
			`INSERT INTO party_commercial.commercial_scope_name (tenant_id, scope_ref, name)
			 VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			`SELECT name FROM party_commercial.commercial_scope_name WHERE tenant_id = $1 AND scope_ref = $2`,
			[]any{tenant.String(), scope, name},
			[]any{tenant.String(), scope},
			name,
		)
		outcome = saved
		return err
	})
	if err != nil {
		return "", fmt.Errorf("save scope name: %w", err)
	}
	return outcome, nil
}

func (catalogue *OperationsCatalogue) SaveDisplayName(
	ctx context.Context,
	tenant domain.TenantID,
	objectID, version, name string,
) (ports.LabelSaveOutcome, error) {
	var outcome ports.LabelSaveOutcome
	err := catalogue.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := catalogue.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		var kind int16
		err = executor.QueryRow(txCtx,
			`SELECT object_kind FROM party_commercial.commercial_version
			  WHERE tenant_id = $1 AND object_id = $2 AND version_label = $3 AND object_kind = $4`,
			tenant.String(), objectID, version, uint8(domain.ServiceProductObject),
		).Scan(&kind)
		if errors.Is(err, pgx.ErrNoRows) {
			outcome = ports.LabelNotFound
			return nil
		}
		if err != nil {
			return err
		}
		saved, err := insertLabel(txCtx, executor,
			`INSERT INTO party_commercial.service_product_display_name
			    (tenant_id, object_kind, object_id, version_label, name)
			 VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
			`SELECT name FROM party_commercial.service_product_display_name
			  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
			[]any{tenant.String(), kind, objectID, version, name},
			[]any{tenant.String(), kind, objectID, version},
			name,
		)
		outcome = saved
		return err
	})
	if err != nil {
		return "", fmt.Errorf("save display name: %w", err)
	}
	return outcome, nil
}

func insertLabel(
	ctx context.Context,
	executor bentopg.Executor,
	insertSQL, selectSQL string,
	insertArgs, selectArgs []any,
	name string,
) (ports.LabelSaveOutcome, error) {
	tag, err := executor.Exec(ctx, insertSQL, insertArgs...)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() > 0 {
		return ports.LabelSaved, nil
	}
	var existing string
	if err := executor.QueryRow(ctx, selectSQL, selectArgs...).Scan(&existing); err != nil {
		return "", err
	}
	if existing == name {
		return ports.LabelAlreadyRegistered, nil
	}
	return ports.LabelContentConflict, nil
}
