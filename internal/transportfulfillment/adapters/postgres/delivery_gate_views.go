package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 交付门禁读数在写事务之前读的两本册（ADR-0213 决定五）：段登记册答对象此刻在哪段、凭什么进的段，交接登记册答进段的
// 那次交接声明了哪个交出节点。
var (
	_ ports.DeliveryGateSegmentView       = (*FulfillmentSegments)(nil)
	_ ports.DeliveryGateEntryHandoverView = (*TransportHandovers)(nil)
)

// FindHandedOverByVersion 按（租户、对象、交接版本）交回`已交接`的各版，供交付门禁读数找对象进段的那次交接
// （ports.DeliveryGateEntryHandoverView，ADR-0213 决定五）。
//
// 交接的键还有一维范围，ADR-0221 之前的存量入场依据却只带版本，所以这里按主键前缀（租户、对象）取、再按版本筛，可能不止
// 一行：如实全交回，哪一版是进段那一次由领域认（FulfillmentParticipation.EnteredByHandover），在 SQL 里再按范围或业务时间
// 筛就是同一条规则的第二个口径。拒收与待确认在 SQL 里就不取——它们立不起参与，这一条是裁决的封闭集，不是认法。读回经同一道重建门。
func (repository *TransportHandovers) FindHandedOverByVersion(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	version domain.HandoverResultVersion,
) ([]domain.TransportHandover, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT scope_ref,
		        released_by, received_by, verdict,
		        releasing_evidence, receiving_evidence, rule_ref, basis_ref,
		        corrects_version, corrected_at, judged_at,
		        releasing_node, receiving_node, onward_node, gate_reading, gate_reading_detail
		   FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND handover_version = $3
		    AND verdict = 'HANDED_OVER'
		  ORDER BY scope_ref`,
		tenant.String(),
		object.String(),
		version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
	}
	defer rows.Close()

	var handovers []domain.TransportHandover
	for rows.Next() {
		var scopeRaw, releasedBy, receivedBy, verdictName string
		var releasing, receiving, rule, basis, corrects *string
		var releasingNode, receivingNode, onwardNode, gateReading *string
		var gateReadingDetail []byte
		var correctedAt *time.Time
		var judgedAt time.Time
		if err := rows.Scan(&scopeRaw, &releasedBy, &receivedBy, &verdictName,
			&releasing, &receiving, &rule, &basis,
			&corrects, &correctedAt, &judgedAt,
			&releasingNode, &receivingNode, &onwardNode, &gateReading, &gateReadingDetail); err != nil {
			return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
		}
		key := ports.TransportHandoverKey{TenantID: tenant, Object: object, Version: version}
		if key.Scope, err = domain.NewHandoverScopeReference(scopeRaw); err != nil {
			return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
		}
		handover, err := rebuildHandover(key, handoverRow{
			releasedBy:    releasedBy,
			receivedBy:    receivedBy,
			verdictName:   verdictName,
			releasing:     releasing,
			receiving:     receiving,
			rule:          rule,
			basis:         basis,
			corrects:      corrects,
			correctedAt:   correctedAt,
			judgedAt:      judgedAt,
			releasingNode: releasingNode,
			receivingNode: receivingNode,
			onwardNode:    onwardNode,
			gateReading:   gateReading,
			gateDetail:    gateReadingDetail,
		})
		if err != nil {
			return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
		}
		handovers = append(handovers, handover)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find handed-over transport handovers by version: %w", err)
	}
	return handovers, nil
}
