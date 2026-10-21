package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	_ ports.ChargeOccurrenceScopeHold = (*ChargeOccurrences)(nil)
	_ ports.ChargeOccurrenceScopeView = (*ChargeOccurrences)(nil)
)

// HoldOccurrenceScope 取（租户、原因、主要业务范围）这一组的事务级咨询锁，与 FindCovering 是同一把（holdChargeOccurrenceScope）。
func (repository *ChargeOccurrences) HoldOccurrenceScope(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.ChargeOccurrenceReason,
	scope domain.OccurrenceScopeReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("hold charge occurrence scope: %w", err)
	}
	return holdChargeOccurrenceScope(ctx, executor, tenant, reason, scope)
}

// holdChargeOccurrenceScope 是发生项范围锁的唯一一处拼法：第一键是锁空间名的散列，第二键是三维以 chr(31) 拼接后的散列，与本仓其余
// 事务级锁同一个双 int 形式。哈希撞上只让两组不相干的范围多等一会儿，不会让同一组的两笔并行。
func holdChargeOccurrenceScope(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	reason domain.ChargeOccurrenceReason,
	scope domain.OccurrenceScopeReference,
) error {
	if tenant.String() == "" || reason.String() == "" || scope.String() == "" {
		return fmt.Errorf("hold charge occurrence scope: scope is incomplete")
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.charge_occurrence.scope'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenant.String(), reason.String(), scope.String(),
	); err != nil {
		return fmt.Errorf("取范围锁：%w", err)
	}
	return nil
}

// ListCurrentOnScope 列出这个范围上各项仍有效的链头（「仍有效」的读法与 FindCovering 同一句），按登记先后排，再逐项按键整条读回、
// 过重建门。范围上的项数是一段上的成员数一级，逐项回读不成问题。
func (repository *ChargeOccurrences) ListCurrentOnScope(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.ChargeOccurrenceReason,
	scope domain.OccurrenceScopeReference,
) ([]ports.ChargeOccurrenceRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT head.occurrence_ref, head.validity_version
		   FROM transport_fulfillment.transport_charge_occurrence AS head
		  WHERE head.tenant_id = $1
		    AND head.reason = $2
		    AND head.scope_ref = $3
		    AND head.revision_kind IS DISTINCT FROM $4
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence AS successor
		         WHERE successor.tenant_id = head.tenant_id
		           AND successor.occurrence_ref = head.occurrence_ref
		           AND successor.corrects_version = head.validity_version)
		  ORDER BY head.recorded_at, head.occurrence_ref`,
		tenant.String(), reason.String(), scope.String(), domain.OccurrenceInvalidated.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
	}
	var keys []ports.ChargeOccurrenceKey
	for rows.Next() {
		var occurrenceRef, version string
		if err := rows.Scan(&occurrenceRef, &version); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
		}
		key := ports.ChargeOccurrenceKey{TenantID: tenant}
		if key.Occurrence, err = domain.NewChargeOccurrenceReference(occurrenceRef); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
		}
		if key.Validity, err = domain.NewOccurrenceValidityVersion(version); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
	}
	records := make([]ports.ChargeOccurrenceRecord, 0, len(keys))
	for _, key := range keys {
		record, found, err := repository.FindByKey(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("list current charge occurrences on scope: %w", err)
		}
		if !found {
			return nil, fmt.Errorf("list current charge occurrences on scope: %s/%s/%s vanished within the same read",
				key.TenantID.String(), key.Occurrence.String(), key.Validity.String())
		}
		records = append(records, record)
	}
	return records, nil
}
