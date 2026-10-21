package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var _ ports.SupplierCostOccurrenceInvalidations = (*SupplierExpectedCosts)(nil)

// LoadOccurrenceCostState 先取（租户、发生项）的事务级咨询锁，再读这一项发生项各条版本链的链头与已失效的那几版（迁移 SA 0044）。锁照本仓
// 其余事务级锁的双 int 形式：第一键是锁空间名的散列，第二键是两维以 chr(31) 拼接后的散列；撞上只让两项不相干的发生项多等一会儿。
// 链头逐版按版本读回，过同一扇重建门。
func (repository *SupplierExpectedCosts) LoadOccurrenceCostState(
	ctx context.Context,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
) (ports.OccurrenceCostState, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.supplier_expected_cost.occurrence'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), occurrence.String(),
	); err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: 取发生项锁：%w", err)
	}

	rows, err := executor.Query(ctx,
		`SELECT head.version
		   FROM settlement_accounting.supplier_expected_cost AS head
		  WHERE head.tenant_id = $1
		    AND head.occurrence_id = $2
		    AND NOT EXISTS (
		        SELECT 1
		          FROM settlement_accounting.supplier_expected_cost AS successor
		         WHERE successor.tenant_id = head.tenant_id
		           AND successor.prior_version = head.version)
		  ORDER BY head.fee_item, head.version`,
		tenant.String(), occurrence.String(),
	)
	if err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
	}
	var versions []domain.SupplierCostVersionID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
		}
		version, err := domain.NewSupplierCostVersionID(raw)
		if err != nil {
			rows.Close()
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
		}
		versions = append(versions, version)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
	}
	var state ports.OccurrenceCostState
	for _, version := range versions {
		head, found, err := repository.LoadExpectedCost(ctx, tenant, version)
		if err != nil {
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
		}
		if !found {
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: version %s vanished within the same transaction", version)
		}
		state.Heads = append(state.Heads, head)
	}

	invalidated, err := executor.Query(ctx,
		`SELECT invalidated_version
		   FROM settlement_accounting.supplier_cost_occurrence_invalidation
		  WHERE tenant_id = $1 AND occurrence_id = $2
		  ORDER BY invalidated_version`,
		tenant.String(), occurrence.String(),
	)
	if err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
	}
	defer invalidated.Close()
	for invalidated.Next() {
		var raw string
		if err := invalidated.Scan(&raw); err != nil {
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
		}
		version, err := domain.NewOccurrenceVersion(raw)
		if err != nil {
			return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
		}
		state.Invalidated = append(state.Invalidated, version)
	}
	if err := invalidated.Err(); err != nil {
		return ports.OccurrenceCostState{}, fmt.Errorf("load occurrence cost state: %w", err)
	}
	return state, nil
}

// RecordOccurrenceInvalidation 记下发生项的这一版已失效；同一（租户、发生项、失效的那一版）再记落回同一行、不报错。
func (repository *SupplierExpectedCosts) RecordOccurrenceInvalidation(
	ctx context.Context,
	tenant domain.TenantID,
	occurrence domain.ChargeOccurrenceID,
	invalidated domain.OccurrenceVersion,
	invalidating domain.OccurrenceVersion,
	recordedAt time.Time,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record occurrence invalidation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_cost_occurrence_invalidation
		     (tenant_id, occurrence_id, invalidated_version, invalidating_version, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT ON CONSTRAINT supplier_cost_occurrence_invalidation_pkey DO NOTHING`,
		tenant.String(), occurrence.String(), invalidated.String(), invalidating.String(), recordedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record occurrence invalidation: %w", err)
	}
	return nil
}
