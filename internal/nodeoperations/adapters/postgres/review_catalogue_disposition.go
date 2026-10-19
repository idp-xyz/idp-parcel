package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ListIdentityReviewUnits 按实物上列待识别实物页（ports.IdentityReviewUnitRow）：有过待识别那一格的作业实物，新近登记在前，每件
// 交回它各条判断链没有后继的那几版（形成格也在——一件实物在另一个节点关联成立的那条链同样左右它的身份）。链头按「没有一行指名它
// 为前版」认，走 reception_predecessor_unique；带收寄的两格的后继只会是这两格，所以在这件实物带收寄的行里就认得全。
//
// 处置文档经领域读回门重建（理由在 ports.IdentityReviewHead 的头注）；其余各格照行转写。
func (catalogue *ReviewCatalogue) ListIdentityReviewUnits(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.IdentityReviewUnitRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list identity review units: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list identity review units: %w", err)
	}
	rows, err := querier.Query(ctx,
		`WITH units AS (
		     SELECT intake ->> 'unit' AS unit, max(recorded_at) AS latest
		       FROM node_operations.reception
		      WHERE tenant_id = $1
		        AND kind = 'PENDING_IDENTIFICATION'
		      GROUP BY intake ->> 'unit'
		      ORDER BY latest DESC, unit
		      LIMIT $2
		 )
		 SELECT units.unit, r.source_id, r.kind,
		        r.intake ->> 'node',
		        COALESCE(r.judgment_version, ''),
		        COALESCE(r.intake ->> 'association', ''),
		        r.candidates,
		        r.identity_conflict,
		        (r.intake ->> 'receivedAt')::timestamptz,
		        r.recorded_at,
		        r.disposition_document
		   FROM units
		   JOIN node_operations.reception AS r
		     ON r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'unit') = units.unit
		  WHERE NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY units.latest DESC, units.unit, r.source_id`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list identity review units: %w", err)
	}
	defer rows.Close()

	list := []ports.IdentityReviewUnitRow{}
	for rows.Next() {
		var (
			unit            string
			head            ports.IdentityReviewHead
			candidatesJSON  []byte
			dispositionJSON []byte
		)
		if err := rows.Scan(
			&unit, &head.SourceID, &head.Kind, &head.Node, &head.JudgmentVersion, &head.Association,
			&candidatesJSON, &head.IdentityConflict, &head.ReceivedAt, &head.RecordedAt, &dispositionJSON,
		); err != nil {
			return nil, fmt.Errorf("list identity review units: %w", err)
		}
		if err := json.Unmarshal(candidatesJSON, &head.Candidates); err != nil {
			return nil, fmt.Errorf("list identity review units: %w", err)
		}
		if len(dispositionJSON) > 0 {
			if head.Disposition, err = domain.ParseIdentityDisposition(dispositionJSON); err != nil {
				return nil, fmt.Errorf("list identity review units: source %q: %w", head.SourceID, err)
			}
		}
		head.ReceivedAt = head.ReceivedAt.UTC()
		head.RecordedAt = head.RecordedAt.UTC()
		if last := len(list) - 1; last >= 0 && list[last].Unit == unit {
			list[last].Heads = append(list[last].Heads, head)
			continue
		}
		list = append(list, ports.IdentityReviewUnitRow{Unit: unit, Heads: []ports.IdentityReviewHead{head}})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list identity review units: %w", err)
	}
	return list, nil
}
