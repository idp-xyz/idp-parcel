package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ListPhysicalControlTransitions 上列节点控制转移册（ADR-0214 决定三；registry=physical-control）：照行转写，新近记录在前，同刻按交接键
// 稳定排序。转出与转入、没推进的那几行同册列出——撤回控制的更正入账「控制来源链待重算」、转入对不上作业实物，人要在这里看得见
// （ADR-0214 决定五、七）。
func (catalogue *ReviewCatalogue) ListPhysicalControlTransitions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PhysicalControlTransitionCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list physical control transitions: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list physical control transitions: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT handover_object, handover_scope, handover_version, direction, outcome, COALESCE(reason, ''),
		        COALESCE(unit, ''), COALESCE(node, ''), COALESCE(control_kind, ''), COALESCE(control_basis, ''),
		        control_established_at, COALESCE(transfer_reference, ''), transferred_at, occurred_at, recorded_at,
		        COALESCE(mismatched_releasing_node, '')
		   FROM node_operations.physical_control_transition
		  WHERE tenant_id = $1
		  ORDER BY recorded_at DESC, handover_object, handover_scope, handover_version, direction
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list physical control transitions: %w", err)
	}
	defer rows.Close()

	list := make([]ports.PhysicalControlTransitionCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row                          ports.PhysicalControlTransitionCatalogueRow
			establishedAt, transferredAt *time.Time
		)
		if err := rows.Scan(
			&row.HandoverObject, &row.HandoverScope, &row.HandoverVersion, &row.Direction, &row.Outcome, &row.Reason,
			&row.Unit, &row.Node, &row.ControlKind, &row.ControlBasis,
			&establishedAt, &row.TransferReference, &transferredAt, &row.OccurredAt, &row.RecordedAt,
			&row.MismatchedReleasingNode,
		); err != nil {
			return nil, fmt.Errorf("list physical control transitions: %w", err)
		}
		row.ControlEstablishedAt = utcInstant(establishedAt)
		row.TransferredAt = utcInstant(transferredAt)
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list physical control transitions: %w", err)
	}
	return list, nil
}

func utcInstant(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}
