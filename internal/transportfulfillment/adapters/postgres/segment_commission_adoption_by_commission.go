package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.SegmentCommissionAdoptionsByCommission = (*SegmentCommissionAdoptions)(nil)

// ListByCommission 按（租户、委托）列出此刻仍采用它的各段，各段只交当前那一代，按段排序（迁移 0035 的按委托索引）。已解除的那几代不列：
// 换版之后的补形成不该再按这份委托去补一段已不按它采购的段，解除之后委托的开始也只按仍采用它的各段重新派生——两处问的是同一件事，
// 「当前」与 FindBySegment 同一个判法（currentAdoptionCondition）。一行装不回就整次报错，不跳过它：跳过一段，换版之后那一段的补形成
// 就无声地缺了，解除之后的开始也会少算一段。
func (repository *SegmentCommissionAdoptions) ListByCommission(
	ctx context.Context,
	key ports.TransportCommissionKey,
) ([]ports.SegmentCommissionAdoptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT adoption.segment_ref, adoption.generation, adoption.commission_id, adoption.provider_ref, adoption.start_basis,
		        adoption.started_at, adoption.content_digest, adoption.recorded_at
		   FROM transport_fulfillment.segment_commission_adoption AS adoption
		  WHERE adoption.tenant_id = $1
		    AND adoption.commission_id = $2
		    AND `+currentAdoptionCondition+`
		  ORDER BY adoption.segment_ref`,
		key.TenantID.String(), key.Commission.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	defer rows.Close()
	var records []ports.SegmentCommissionAdoptionRecord
	for rows.Next() {
		record, err := scanAdoption(key.TenantID, rows)
		if err != nil {
			return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list segment commission adoptions by commission: %w", err)
	}
	return records, nil
}
