package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把渠道产品目录读端口（ADR-0077）挂在 OperationsCatalogue 上：管理台
// channel-product-catalog 页的供数面。上列对象是各映射的**最新登记修订**；修订史
// 是登记册的证据面，不是目录的行（判据同参与方身份目录）。
var _ ports.ProductChannelMappingCatalogueRead = (*OperationsCatalogue)(nil)

// ListProductChannelMappings 上列产品—渠道映射的最新修订。
//
// channel_refs 原样转写：空数组即显式“未配置”绑定，页面据此如实显示，不是数据缺件
// （0016 表注）。行上不导出状态——映射没有独立状态代数，是否参与新的渠道决策由
// 消费方对区间判断（ports.ProductChannelMappingRow 上的裁决）。
func (catalogue *OperationsCatalogue) ListProductChannelMappings(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ProductChannelMappingRow, error) {
	if err := requirePositiveLimit("list product channel mappings", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list product channel mappings: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT mapping.tenant_id, mapping.mapping_id, mapping.revision,
		        mapping.product_object_id, mapping.product_version_label,
		        mapping.channel_refs, mapping.basis_ref,
		        mapping.effective_starts_at, mapping.effective_ends_at,
		        mapping.recorded_at
		   FROM (
		        SELECT DISTINCT ON (mapping_id) *
		          FROM party_commercial.product_channel_mapping_registration
		         WHERE tenant_id = $1
		         ORDER BY mapping_id, revision DESC
		   ) AS mapping
		  ORDER BY mapping.recorded_at DESC, mapping.mapping_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list product channel mappings: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.ProductChannelMappingRow, 0, limit)
	for rows.Next() {
		var row ports.ProductChannelMappingRow
		var channelRefs []byte
		var endsAt *time.Time
		if err := rows.Scan(
			&row.TenantID, &row.MappingID, &row.Revision,
			&row.ProductObjectID, &row.ProductVersionLabel,
			&channelRefs, &row.Basis,
			&row.EffectiveStartsAt, &endsAt,
			&row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list product channel mappings: %w", err)
		}
		channels := make([]string, 0, 2)
		if err := json.Unmarshal(channelRefs, &channels); err != nil {
			return nil, fmt.Errorf("list product channel mappings: 渠道绑定格不是数组：%w", err)
		}
		row.Channels = channels
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list product channel mappings: %w", err)
	}
	return catalogueRows, nil
}
