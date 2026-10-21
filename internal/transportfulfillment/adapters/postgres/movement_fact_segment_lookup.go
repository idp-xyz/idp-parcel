package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.MovementFactsOfSegment = (*MovementFacts)(nil)

// FindBySegment 列出声明了这一段的全部在册版本（ADR-0224 决定二）。按事实分组、组内按登记时刻排，同一时刻再按版本串排，
// 好让次序在重放时一字不变。段只是登记方声明的引用、没有外键（迁移 0026），所以本口不问段在不在册：段不在册时照样交回声明
// 了它的事实，那正是补派生要找的那一批。
//
// 一行装不回就整次报错，不跳过那一行：跳过会让补派生把「漏了一条」当成「全补完了」。
func (repository *MovementFacts) FindBySegment(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
) ([]ports.MovementFactRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find movement facts of segment: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+movementFactColumns+`
		   FROM transport_fulfillment.transport_movement_fact
		  WHERE tenant_id = $1 AND segment_ref = $2
		  ORDER BY fact_ref, recorded_at, version`,
		key.TenantID.String(), key.Segment.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find movement facts of segment: %w", err)
	}
	defer rows.Close()

	records := []ports.MovementFactRecord{}
	for rows.Next() {
		var row movementFactRow
		if err := rows.Scan(row.targets()...); err != nil {
			return nil, fmt.Errorf("find movement facts of segment: %w", err)
		}
		record, err := row.record(key.TenantID)
		if err != nil {
			return nil, fmt.Errorf("find movement facts of segment: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find movement facts of segment: %w", err)
	}
	return records, nil
}
