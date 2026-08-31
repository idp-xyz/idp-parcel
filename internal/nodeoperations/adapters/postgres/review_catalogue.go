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
		        COALESCE(control->>'releasedBy', ''),
		        (control->>'releasedAt')::timestamptz,
		        service_markers,
		        recorded_at
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
		)
		if err := rows.Scan(
			&row.SourceID, &row.Kind,
			&row.Unit, &row.Node, &row.DeliveredBy, &row.ReceivedAt,
			&row.ControlKind, &row.ControlEstablishedAt,
			&row.ControlReleasedBy, &releasedAt,
			&markersJSON, &row.RecordedAt,
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
// 的身份视角，新近登记在前。
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
		`SELECT source_id,
		        intake->>'unit',
		        intake->>'node',
		        candidates,
		        identity_conflict,
		        (intake->>'receivedAt')::timestamptz,
		        COALESCE(intake->>'association', ''),
		        recorded_at
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND kind = 'PENDING_IDENTIFICATION'
		  ORDER BY recorded_at DESC, source_id
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
		)
		if err := rows.Scan(
			&row.SourceID, &row.Unit, &row.Node,
			&candidatesJSON, &row.IdentityConflict,
			&row.ReceivedAt, &row.Association, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list unidentified items: %w", err)
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
		        COALESCE(snapshots -> -1 ->> 'seal', ''),
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
			&row.LatestSeal, &latestSealedAt, &closedAt,
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
