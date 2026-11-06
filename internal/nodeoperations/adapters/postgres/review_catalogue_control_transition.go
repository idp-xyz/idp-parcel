package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

const physicalControlTransitionRegister = "node_operations.physical_control_transition"

// physicalControlTransitionPages 把 ports.PhysicalControlTransitionPages 落到转移册的列上：行标识是主键去掉租户的四段；q 在交接键
// 三段、作业实物与节点上做字面包含（对不上实物、没带控制的行没有后两格，只靠交接键命中）。
var physicalControlTransitionPages = ports.PhysicalControlTransitionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"handover_object", "handover_scope", "handover_version", "direction"},
	Filters:  map[string]string{"outcome": "outcome"},
	Keyword:  []string{"handover_object", "handover_scope", "handover_version", "unit", "node"},
})

// ListPhysicalControlTransitions 上列节点控制转移册（ADR-0214 决定三；registry=physical-control）的一页：照行转写，缺省新近记录在前，
// 同刻按交接键倒序。转出与转入、没推进的那几行同册列出——撤回控制的更正入账「控制来源链待重算」、转入对不上作业实物，人要在这里
// 看得见（ADR-0214 决定五、七）。
func (catalogue *ReviewCatalogue) ListPhysicalControlTransitions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PhysicalControlTransitionCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.PhysicalControlTransitionCatalogueRow], error) {
		return cataloguepage.Result[ports.PhysicalControlTransitionCatalogueRow]{}, fmt.Errorf("list physical control transitions: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := physicalControlTransitionPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(physicalControlTransitionRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`handover_object, handover_scope, handover_version, direction, outcome, COALESCE(reason, ''),
		 COALESCE(unit, ''), COALESCE(node, ''), COALESCE(control_kind, ''), COALESCE(control_basis, ''),
		 control_established_at, COALESCE(transfer_reference, ''), transferred_at, occurred_at, recorded_at,
		 COALESCE(mismatched_releasing_node, '')`,
		physicalControlTransitionRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.PhysicalControlTransitionCatalogueRow, 0, limit+1)
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
			return fail(err)
		}
		row.ControlEstablishedAt = utcInstant(establishedAt)
		row.TransferredAt = utcInstant(transferredAt)
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.PhysicalControlTransitionCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.RecordedAt),
				Identity: []string{row.HandoverObject, row.HandoverScope, row.HandoverVersion, row.Direction},
			}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func utcInstant(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}
