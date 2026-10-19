package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ReviewCatalogue 实现 ports.ReviewCatalogueRead：节点作业查阅页的列表读面
// （ADR-0077，票 admin-skeleton-closure-batch/05）。
//
// 目录上列是照实转写，不经领域重建门：重建是写路与按键读回的纪律（坏行要在那里
// 响亮），检索列面把登记的字段原样透出。jsonb 字段在 SQL 里就地展开成列
// （intake/control 的键形状由本包写侧的 intakeRow/controlRow 固定），时刻经
// ::timestamptz 还原——写侧以 RFC 3339 序列化，两侧共一份形状，不在 Go 里二次解码。
// 例外在 ListConsolidationMembers：方向依据记录经领域重建门读回，理由在 ports.ReviewCatalogueRead 所在文件的头注。
//
// 所有语句显式携带租户条件（ADR-0003）；读走 ReadExecutor，事务外用显式注入的
// 连接池。
type ReviewCatalogue struct {
	db *bentopg.DB
}

var _ ports.ReviewCatalogueRead = (*ReviewCatalogue)(nil)

func NewReviewCatalogue(db *bentopg.DB) (*ReviewCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ReviewCatalogue{db: db}, nil
}

// ListReceptions 上列收寄登记册：只列带收寄与控制在场的两格（kind 封闭集见端口注
// 释），新近登记在前，同刻按来源标识稳定排序。
func (catalogue *ReviewCatalogue) ListReceptions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ReceptionCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list receptions: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list receptions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT source_id, kind,
		        intake->>'unit',
		        intake->>'node',
		        intake->>'deliveredBy',
		        (intake->>'receivedAt')::timestamptz,
		        control->>'kind',
		        (control->>'establishedAt')::timestamptz,
		        COALESCE(`+foldedControl+`->>'releasedBy', ''),
		        (`+foldedControl+`->>'releasedAt')::timestamptz,
		        service_markers,
		        service_result_check,
		        recorded_at,
		        identity_check,
		        delivery_scope_check, delivery_scope_unverified_reason, delivery_scope_inapplicability
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND kind IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION')
		  ORDER BY recorded_at DESC, source_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list receptions: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ReceptionCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row         ports.ReceptionCatalogueRow
			releasedAt  *time.Time
			markersJSON []byte
			checkName   *string
			identity    *string
			scope       [3]*string
		)
		if err := rows.Scan(
			&row.SourceID, &row.Kind,
			&row.Unit, &row.Node, &row.DeliveredBy, &row.ReceivedAt,
			&row.ControlKind, &row.ControlEstablishedAt,
			&row.ControlReleasedBy, &releasedAt,
			&markersJSON, &checkName, &row.RecordedAt, &identity,
			&scope[0], &scope[1], &scope[2],
		); err != nil {
			return nil, fmt.Errorf("list receptions: %w", err)
		}
		if releasedAt != nil {
			utc := releasedAt.UTC()
			row.ControlReleasedAt = &utc
		}
		if err := json.Unmarshal(markersJSON, &row.ServiceMarkers); err != nil {
			return nil, fmt.Errorf("list receptions: %w", err)
		}
		// 核对态与按键读回共用 serviceResultCheckFrom：词表只认一份，集外取值让整页答不出，不悄悄折成「不带」——
		// 形成行缺了这一格，审阅者又分不出标记的空是查过了还是没读到。
		check, err := serviceResultCheckFrom(checkName)
		if err != nil {
			return nil, fmt.Errorf("list receptions: source %q: %w", row.SourceID, err)
		}
		row.ServiceResultCheck = check
		// 身份核对态同理，与按键读回共用 identityCheckFrom；交付范围核对态与按键读回共用 deliveryScopeCheckFrom。
		if row.IdentityCheck, err = identityCheckFrom(identity); err != nil {
			return nil, fmt.Errorf("list receptions: source %q: %w", row.SourceID, err)
		}
		if row.DeliveryScopeCheck, err = deliveryScopeCheckFrom(scope[0], scope[1], scope[2]); err != nil {
			return nil, fmt.Errorf("list receptions: source %q: %w", row.SourceID, err)
		}
		row.ReceivedAt = row.ReceivedAt.UTC()
		row.ControlEstablishedAt = row.ControlEstablishedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list receptions: %w", err)
	}
	return list, nil
}

// ListUnidentifiedItems 上列待识别实物册：收寄登记里 PENDING_IDENTIFICATION 那一格
// 的身份视角，新近登记在前。来源文档里的号码照行转写（来源文档为空即来源未保全）；直接接续这一版的那一版按（租户，前版）
// 取，走 reception_predecessor_unique。
func (catalogue *ReviewCatalogue) ListUnidentifiedItems(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.UnidentifiedItemCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list unidentified items: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unidentified items: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT r.source_id,
		        r.intake->>'unit',
		        r.intake->>'node',
		        r.candidates,
		        r.identity_conflict,
		        (r.intake->>'receivedAt')::timestamptz,
		        COALESCE(r.intake->>'association', ''),
		        r.recorded_at,
		        COALESCE(r.judgment_version, ''),
		        r.source_document IS NOT NULL,
		        COALESCE(r.source_document->>'mark', ''),
		        COALESCE((SELECT successor.judgment_version
		                    FROM node_operations.reception AS successor
		                   WHERE successor.tenant_id = r.tenant_id
		                     AND successor.predecessor_version = r.judgment_version), ''),
		        r.identity_check,
		        r.identity_conflict_cause,
		        r.delivery_scope_check, r.delivery_scope_unverified_reason, r.delivery_scope_inapplicability
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.kind = 'PENDING_IDENTIFICATION'
		  ORDER BY r.recorded_at DESC, r.source_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unidentified items: %w", err)
	}
	defer rows.Close()

	list := make([]ports.UnidentifiedItemCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row            ports.UnidentifiedItemCatalogueRow
			candidatesJSON []byte
			identity       *string
			conflictCause  *string
			scope          [3]*string
		)
		if err := rows.Scan(
			&row.SourceID, &row.Unit, &row.Node,
			&candidatesJSON, &row.IdentityConflict,
			&row.ReceivedAt, &row.Association, &row.RecordedAt,
			&row.JudgmentVersion, &row.SourcePreserved, &row.Mark, &row.SucceededBy,
			&identity, &conflictCause,
			&scope[0], &scope[1], &scope[2],
		); err != nil {
			return nil, fmt.Errorf("list unidentified items: %w", err)
		}
		check, err := identityCheckFrom(identity)
		if err != nil {
			return nil, fmt.Errorf("list unidentified items: source %q: %w", row.SourceID, err)
		}
		row.IdentityCheck = check
		if row.IdentityConflictCause, err = identityConflictCauseFrom(conflictCause); err != nil {
			return nil, fmt.Errorf("list unidentified items: source %q: %w", row.SourceID, err)
		}
		if row.DeliveryScopeCheck, err = deliveryScopeCheckFrom(scope[0], scope[1], scope[2]); err != nil {
			return nil, fmt.Errorf("list unidentified items: source %q: %w", row.SourceID, err)
		}
		if err := json.Unmarshal(candidatesJSON, &row.Candidates); err != nil {
			return nil, fmt.Errorf("list unidentified items: %w", err)
		}
		row.ReceivedAt = row.ReceivedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list unidentified items: %w", err)
	}
	return list, nil
}

// ListConsolidationUnits 上列集运单元登记册。行上没有业务时刻可排序（形成时间无
// 登记格，库面簿记时刻不是业务事实），按实例标识稳定排序——同一册两次读回同一个序。
// 最近封签在 SQL 里取快照数组末元素：写侧按封装次序追加快照，末元素即最近一次。
func (catalogue *ReviewCatalogue) ListConsolidationUnits(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ConsolidationUnitCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list consolidation units: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list consolidation units: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT unit_id, asset_ref, phase,
		        jsonb_array_length(members),
		        jsonb_array_length(snapshots),
		        opened_source ->> 'sourceId',
		        opened_source ->> 'performedBy',
		        COALESCE(snapshots -> -1 ->> 'seal', ''),
		        COALESCE(snapshots -> -1 -> 'source' ->> 'sourceId', ''),
		        COALESCE(snapshots -> -1 -> 'source' ->> 'performedBy', ''),
		        (snapshots -> -1 ->> 'sealedAt')::timestamptz,
		        closed_at
		   FROM node_operations.consolidation_unit
		  WHERE tenant_id = $1
		  ORDER BY unit_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list consolidation units: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ConsolidationUnitCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row            ports.ConsolidationUnitCatalogueRow
			latestSealedAt *time.Time
			closedAt       *time.Time
		)
		if err := rows.Scan(
			&row.UnitID, &row.Asset, &row.Phase,
			&row.MemberCount, &row.SealCount,
			&row.OpenedSourceID, &row.OpenedBy,
			&row.LatestSeal, &row.LatestSealSourceID, &row.LatestSealPerformedBy,
			&latestSealedAt, &closedAt,
		); err != nil {
			return nil, fmt.Errorf("list consolidation units: %w", err)
		}
		if latestSealedAt != nil {
			utc := latestSealedAt.UTC()
			row.LatestSealedAt = &utc
		}
		if closedAt != nil {
			utc := closedAt.UTC()
			row.ClosedAt = &utc
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list consolidation units: %w", err)
	}
	return list, nil
}

// ListConsolidationMembers 上列「集运成员关系」册：移入与移出两种来源事实，按记录时刻倒序（同刻按来源标识）。
// 方向依据记录经领域重建门读回，与按键读回走同一份行模型；一行坏记录让整页答不出，而不是悄悄显示成依据齐全。
func (catalogue *ReviewCatalogue) ListConsolidationMembers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ConsolidationMemberCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list consolidation members: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list consolidation members: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT source_id, action, unit_id, member_id, performed_by, evidence,
		        occurred_at, recorded_at, directional_basis
		   FROM node_operations.consolidation_fact
		  WHERE tenant_id = $1
		    AND action IN ('ADD_MEMBER', 'REMOVE_MEMBER')
		  ORDER BY recorded_at DESC, source_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list consolidation members: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ConsolidationMemberCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row   ports.ConsolidationMemberCatalogueRow
			basis []byte
		)
		if err := rows.Scan(
			&row.SourceID, &row.Action, &row.UnitID, &row.Member, &row.PerformedBy, &row.Evidence,
			&row.OccurredAt, &row.RecordedAt, &basis,
		); err != nil {
			return nil, fmt.Errorf("list consolidation members: %w", err)
		}
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		if basis != nil {
			record, err := rebuildDirectionalBasis(basis)
			if err != nil {
				return nil, fmt.Errorf("list consolidation members: source %q: directional basis: %w", row.SourceID, err)
			}
			row.DirectionalBasis = record
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list consolidation members: %w", err)
	}
	return list, nil
}
